package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/kurrik/oauth1a"
	"github.com/kurrik/twittergo"
	"io"
	"log"
	mrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const mickensSource = "parsed-mickens.txt"

var mickens []string

func seed() {
	var seed = make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, seed)
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
	}
	mrand.Seed(int64(binary.BigEndian.Uint64(seed)))
}

func loadMickens() ([]string, error) {
	file, err := os.Open(mickensSource)
	if err != nil {
		return nil, err
	}

	var sentences []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if len(s) == 0 {
			continue
		}
		sentences = append(sentences, s)
	}
	return sentences, nil
}

func splitForTweet(in string) []string {
	var tweets []string
	var words = strings.Split(in, " ")

	for {
		var tweet = ""
		for {
			tweet += words[0]
			words = words[1:]
			if len(words) == 0 {
				tweets = append(tweets, tweet)
				break
			} else if len(tweet)+len(words[0]) > 138 {
				tweets = append(tweets, tweet)
				break
			}
			tweet += " "
		}
		if len(words) == 0 {
			break
		}
	}
	return tweets
}

func LoadCredentials() (client *twittergo.Client, err error) {
	config := &oauth1a.ClientConfig{
		ConsumerKey:    os.Getenv("CONSUMER_KEY"),
		ConsumerSecret: os.Getenv("CONSUMER_SECRET"),
	}
	user := oauth1a.NewAuthorizedConfig(os.Getenv("API_KEY"), os.Getenv("API_SECRET"))
	client = twittergo.NewClient(config, user)
	return
}

func postTweet(status string) error {
	var (
		err    error
		client *twittergo.Client
		req    *http.Request
		resp   *twittergo.APIResponse
		tweet  *twittergo.Tweet
	)
	client, err = LoadCredentials()
	if err != nil {
		fmt.Printf("Could not parse CREDENTIALS file: %v\n", err)
		os.Exit(1)
	}
	data := url.Values{}
	data.Set("status", status)
	body := strings.NewReader(data.Encode())
	req, err = http.NewRequest("POST", "/1.1/statuses/update.json", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.SendRequest(req)
	if err != nil {
		return err
	}
	tweet = &twittergo.Tweet{}
	err = resp.Parse(tweet)
	if err != nil {
		if rle, ok := err.(twittergo.RateLimitError); ok {
			fmt.Printf("Rate limited, reset at %v\n", rle.Reset)
		} else if errs, ok := err.(twittergo.Errors); ok {
			for i, val := range errs.Errors() {
				fmt.Printf("Error #%v - ", i+1)
				fmt.Printf("Code: %v ", val.Code())
				fmt.Printf("Msg: %v\n", val.Message())
			}
		} else {
			fmt.Printf("Problem parsing response: %v\n", err)
		}
	}
	return err
}

func httpReload(w http.ResponseWriter, r *http.Request) {
	log.Printf("request from %#v", *r)
	newMickens, err := loadMickens()
	if err != nil {
		log.Printf("error reloading database: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	} else {
		mickens = newMickens
		w.Write([]byte("OK"))
	}
}

func httpTickle(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}

func server() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	// http.HandleFunc("/reload", httpReload)
	http.HandleFunc("/tickle", httpTickle)
	log.Println("starting server")
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	var err error
	seed()
	mickens, err = loadMickens()
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}

	go server()
	go func() {
		for {
			delay := time.Duration(mrand.Int63()%7200 + 3600)
			delay *= 1000000000

			var tweet string
			for {
				i := mrand.Int() % (len(mickens))
				tweet = mickens[i]
				if len(tweet) > 140 {
					continue
				}
				break
			}
			err := postTweet(tweet)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
			}
			<-time.After(delay)
		}
	}()
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Kill, os.Interrupt, syscall.SIGTERM)
	<-sigc
	log.Println("shutting down.")
}
