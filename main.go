package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"sync"
	"text/template"
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

	log.Println("Reading profile")
	f, err := ioutil.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatalln(err)
	}
	if err := json.Unmarshal(f, &profile); err != nil {
		log.Fatalln(err)
	}

	log.Println("Reading history")
	f, err = ioutil.ReadFile(profile.HistFile)
	if err == nil {
		json.Unmarshal(f, &history)
	} else if os.IsNotExist(err) {
		log.Printf("History file (%s) not found, it will be created",
			profile.HistFile)
	} else {
		log.Fatalln(err)
	}

	log.Println("Fetching feeds")
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
	date, _ := item.ParsedPubDate()
	data := struct {
		SubjectPrefix, ChanTitle, ItemTitle string
		Date                                time.Time
		Links                               []*rss.Link
		Description                         string
		Content                             *rss.Content
	}{profile.SubjectPrefix, ch.Title, item.Title, date,
		item.Links, item.Description, item.Content}

	t, err := template.New("mail").Parse(mailTmpl)
	if err != nil {
		return err
	}
	msg := new(bytes.Buffer)
	if err := t.Execute(msg, data); err != nil {
		return err
	}

	log.Printf("Sending e-mail: [%s] %s", ch.Title, item.Title)
	auth := smtp.PlainAuth("", profile.SmtpUser, profile.SmtpPass, profile.SmtpHost)
	err = smtp.SendMail(profile.SmtpAddr, auth, profile.SrcEmail,
		[]string{profile.DstEmail}, msg.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func writeHist() {
	log.Println("Updating history file")
	m, err := json.Marshal(history)
	if err != nil {
		errChan <- err
	}
	ioutil.WriteFile(profile.HistFile, m, 0600)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gfm profile_file")
}

const mailTmpl = `Subject: {{.SubjectPrefix}} [{{.ChanTitle}}] {{.ItemTitle}}
MIME-version: 1.0;
Content-Type: text/html; charset="UTF-8";

<b>Title:</b> {{.ItemTitle}}<br>
{{if not .Date.IsZero}}<b>Date:</b> {{.Date.Format "2 January 2006 15:04"}}<br>{{end}}
<b>Links:</b><br>
{{range .Links}}
  - {{.Href}}<br>
{{end}}
{{if .Description}}<b>Description:</b><br>{{.Description}}<br>{{end}}
{{if .Content}}<b>Content:</b><br>{{.Content.Text}}{{end}}
`
