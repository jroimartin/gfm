package feedmailer

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"text/template"
	"time"

	rss "github.com/jteeuwen/go-pkg-rss"
)

type profile struct {
	SmtpAddr      string
	SmtpUser      string
	SmtpPass      string
	SmtpHost      string
	SrcEmail      string
	DstEmails     []string
	SubjectPrefix string
	FetchTimeout  int
	HistFile      string
	Feeds         []string
}

type histItem struct {
	url        string
	lastUpdate time.Time
}

type FeedMailer struct {
	ErrChan  chan error
	histChan chan histItem
	prof     profile
	history  map[string]time.Time
}

func NewFeedMailer() *FeedMailer {
	fm := &FeedMailer{}
	fm.ErrChan = make(chan error)
	fm.histChan = make(chan histItem)
	fm.history = make(map[string]time.Time)
	return fm
}

func (fm *FeedMailer) Start(file string) error {
	log.Println("Reading profile")
	f, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(f, &fm.prof); err != nil {
		return err
	}

	log.Println("Reading history")
	f, err = ioutil.ReadFile(fm.prof.HistFile)
	if err == nil {
		json.Unmarshal(f, &fm.history)
	} else if os.IsNotExist(err) {
		log.Printf("History file (%s) not found, it will be created", fm.prof.HistFile)
	} else {
		return err
	}

	go fm.updateHistory()

	log.Println("Fetching feeds")
	for _, url := range fm.prof.Feeds {
		go fm.fetch(url)
	}

	return nil
}

func (fm *FeedMailer) fetch(url string) {
	feed := rss.New(fm.prof.FetchTimeout, true, fm.chanHandler, fm.itemHandler)
	for {
		if err := feed.Fetch(url, nil); err != nil {
			log.Println(err)
		}
		<-time.After(time.Duration(feed.SecondsTillUpdate()) * time.Second)
	}
}

func (fm *FeedMailer) chanHandler(feed *rss.Feed, newChannels []*rss.Channel) {
	log.Printf("%d new channel(s) in %s\n", len(newChannels), feed.Url)
}

func (fm *FeedMailer) itemHandler(feed *rss.Feed, ch *rss.Channel, newItems []*rss.Item) {
	log.Printf("%d new item(s) in %s\n", len(newItems), feed.Url)

	var lastUpdate time.Time
	for _, item := range newItems {
		itemDate, err := item.ParsedPubDate()
		if err != nil {
			fm.ErrChan <- err
		}
		if fm.history[feed.Url].IsZero() || itemDate.After(fm.history[feed.Url]) {
			if err := fm.mail(ch, item); err != nil {
				fm.ErrChan <- err
			}
			if itemDate.After(lastUpdate) {
				lastUpdate = itemDate
			}
		}
	}

	if !lastUpdate.IsZero() {
		fm.histChan <- histItem{feed.Url, lastUpdate}
	}
}

func (fm *FeedMailer) mail(ch *rss.Channel, item *rss.Item) error {
	date, _ := item.ParsedPubDate()
	data := struct {
		SubjectPrefix, ChanTitle, ItemTitle string
		Date                                time.Time
		Links                               []*rss.Link
		Description                         string
		Content                             *rss.Content
	}{fm.prof.SubjectPrefix, ch.Title, item.Title, date,
		item.Links, item.Description, item.Content}

	msg := &bytes.Buffer{}
	if err := mailTemplate.Execute(msg, data); err != nil {
		return err
	}

	log.Printf("Sending e-mail: [%s] %s", ch.Title, item.Title)
	auth := smtp.PlainAuth("", fm.prof.SmtpUser, fm.prof.SmtpPass, fm.prof.SmtpHost)
	err := smtp.SendMail(fm.prof.SmtpAddr, auth, fm.prof.SrcEmail,
		fm.prof.DstEmails, msg.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (fm *FeedMailer) updateHistory() {
	for {
		hist := <-fm.histChan
		log.Println("Updating history file")
		fm.history[hist.url] = hist.lastUpdate
		buf, err := json.Marshal(fm.history)
		if err != nil {
			fm.ErrChan <- err
		}
		if err := ioutil.WriteFile(fm.prof.HistFile, buf, 0600); err != nil {
			fm.ErrChan <- err
		}
	}
}

var mailTemplate = template.Must(template.New("mail").Parse(mailMessage))

const mailMessage = `Subject: {{.SubjectPrefix}} [{{.ChanTitle}}] {{.ItemTitle}}
MIME-version: 1.0;
Content-Type: text/html; charset="UTF-8";

<b>Title:</b> {{.ItemTitle}}<br>
{{if not .Date.IsZero}}<b>Date:</b> {{.Date.Format "2 January 2006 15:04"}}<br>{{end}}
{{if .Links}}
<b>Links:</b><br>
{{range .Links}}
  - {{.Href}}<br>
{{end}}
{{end}}
{{if .Description}}<b>Description:</b><br>{{.Description}}<br>{{end}}
{{if .Content}}<b>Content:</b><br>{{.Content.Text}}{{end}}`
