GFM - Go Feed Mailer
====================

Service that checks for updates on your RSS feeds and send them to you by e-mail.

Installation
------------
	$ go get github.com/jroimartin/gfm

Usage
-----
	$ gfm profile_file &> profile.log &

Profile example
---------------
	$ cat profile_example
	{
		"SmtpAddr": "smtp.example.com:587",
		"SmtpUser": "username",
		"SmtpPass": "password",
		"SmtpHost": "smtp.example.com",
		"SrcEmail": "src@example.com",
		"DstEmail": "dst@example.com",
		"SubjectPrefix": "[RSS]",
		"FetchTimeout": 5,
		"HistFile": "profile_history",
		"Feeds": [
			"http://www.reddit.com/r/golang/.rss",
			"http://blog.gopheracademy.com/feed.atom"
		]
	}
