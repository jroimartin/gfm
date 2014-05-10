package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"sync"
	"time"

	rss "github.com/jteeuwen/go-pkg-rss"
)

type Profile struct {
	SmtpAddr      string
	SmtpUser      string
	SmtpPass      string
	SmtpHost      string
	SrcEmail      string
	DstEmail      string
	SubjectPrefix string
	FetchTimeout  int
	HistFile      string
	Feeds         []string
}

var profile Profile
var history map[string]time.Time
var errChan chan error
var mutex sync.Mutex

func init() {
	errChan = make(chan error)
	history = make(map[string]time.Time)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	log.Println("Reading profile...")
	f, err := ioutil.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalln(err)
	}
	if err := json.Unmarshal(f, &profile); err != nil {
		log.Fatalln(err)
	}

	log.Println("Reading history...")
	f, err = ioutil.ReadFile(profile.HistFile)
	if err == nil {
		json.Unmarshal(f, &history)
	} else if os.IsNotExist(err) {
		log.Printf("History file (%s) not found, it will be created",
			profile.HistFile)
	} else {
		log.Fatalln(err)
	}

	log.Println("Fetching feeds...")
	for _, url := range profile.Feeds {
		go fetch(url)
	}
	log.Fatalln(<-errChan)
}

func fetch(url string) {
	feed := rss.New(profile.FetchTimeout, true, chanHandler, itemHandler)
	for {
		if err := feed.Fetch(url, nil); err != nil {
			errChan <- err
		}
		<-time.After(time.Duration(feed.SecondsTillUpdate()) * time.Second)
	}
}

func chanHandler(feed *rss.Feed, newChannels []*rss.Channel) {
	log.Printf("%d new channel(s) in %s\n", len(newChannels), feed.Url)
}

func itemHandler(feed *rss.Feed, ch *rss.Channel, newItems []*rss.Item) {
	log.Printf("%d new item(s) in %s\n", len(newItems), feed.Url)

	var lastUpdate time.Time
	for _, item := range newItems {
		itemDate, err := item.ParsedPubDate()
		if err != nil {
			errChan <- err
		}
		if history[feed.Url].IsZero() || itemDate.After(history[feed.Url]) {
			if err := mail(ch, item); err != nil {
				errChan <- err
			}
			if itemDate.After(lastUpdate) {
				lastUpdate = itemDate
			}
		}
	}

	if !lastUpdate.IsZero() {
		mutex.Lock()
		history[feed.Url] = lastUpdate
		writeHist()
		mutex.Unlock()
	}
}

func mail(ch *rss.Channel, item *rss.Item) error {
	subject := fmt.Sprintf("Subject: %s [%s] %s\n",
		profile.SubjectPrefix, ch.Title, item.Title)
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n";
	body := fmt.Sprintf("<b>Title</b>: %s<br>\n", item.Title)
	if date, err := item.ParsedPubDate(); err == nil {
		body += fmt.Sprintf("<b>Date</b>: %v<br>\n", date)
	}
	body += fmt.Sprintf("<b>Links:</b><br>\n")
	for _, link := range item.Links {
		body += fmt.Sprintf("  - %s<br>\n", link.Href)
	}
	body += fmt.Sprintf("<b>Description:</b><br>\n%s<br>\n", item.Description)
	if item.Content != nil {
		body += fmt.Sprintf("<b>Content:</b><br>\n%s", item.Content.Text)
	}

	log.Println("Sending e-mail...", subject)
	auth := smtp.PlainAuth("", profile.SmtpUser, profile.SmtpPass, profile.SmtpHost)
	err := smtp.SendMail(profile.SmtpAddr, auth, profile.SrcEmail,
		[]string{profile.DstEmail}, []byte(subject+mime+body))
	if err != nil {
		return err
	}
	return nil
}

func writeHist() {
	log.Println("Updating history file...")
	m, err := json.Marshal(history)
	if err != nil {
		errChan <- err
	}
	ioutil.WriteFile(profile.HistFile, m, 0600)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gfm profile_file")
}
