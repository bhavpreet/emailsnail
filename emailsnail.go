package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Concurrency management for number of go threads
var POOL_SIZE = 10

var DEPTH = 2
var email_regex, _ = regexp.Compile(`[a-z0-9._%+\-]+(@|&#64;)[a-z0-9.\-]+(\.|&#46;)[a-z]{2,4}`)

type eRet struct {
	master_url string
	email      string
}

// CRAWL -> FIND ABOUT/CONTACT -> EXTRACT

// Helper function to pull the href attribute from a Token
func getHref(t html.Token) (ok bool, href string) {
	// Iterate over all of the Token's attributes until we find an "href"
	for _, a := range t.Attr {
		if a.Key == "href" {
			href = a.Val
			ok = true
		}
	}

	// "bare" return will return the variables (ok, href) as defined in
	// the function definition
	return
}

// Extract all http** links from a given webpage
func crawl(master_url string, url string, emails chan eRet, ch chan string, chFinished chan bool) {
	resp, err := http.Get(url)

	defer func() {
		// Notify that we're done after this function
		chFinished <- true
	}()

	if err != nil {
		fmt.Println("ERROR: Failed to crawl \"" + url + "\"")
		return
	}

	b := resp.Body
	defer b.Close() // close Body when the function returns

	z := html.NewTokenizer(b)

	for {
		tt := z.Next()

		for _, e := range email_regex.FindAllStringSubmatch(string(z.Raw()), -1) {
			if strings.Index(strings.ToLower(e[0]), ".jpg") == -1 && strings.Index(strings.ToLower(e[0]), ".png") == -1 {
				emails <- eRet{master_url, e[0]}
			}
		}

		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			return
		case tt == html.TextToken:

		case tt == html.StartTagToken:
			t := z.Token()

			// Check if the token is an <a> tag
			isAnchor := t.Data == "a"
			if !isAnchor {
				continue
			}

			// Extract the href value, if there is one
			ok, url := getHref(t)
			if !ok {
				continue
			}

			// Make sure the url begines in http**
			hasProto := strings.Index(url, "http") == 0
			if hasProto {
				// ch <- url
				if strings.Index(strings.ToLower(url), "contact") > -1 || strings.Index(strings.ToLower(url), "about") > -1 || strings.Index(strings.ToLower(url), "support") > -1 || strings.Index(strings.ToLower(url), "work") > -1 {
					ch <- url
				}
			}
		}
	}
}

func extract_emails(master_url string, url string, done chan string, depth int, emails chan eRet) {

	var go_pool = make(chan bool, POOL_SIZE)
	// Channels
	chUrls := make(chan string, POOL_SIZE)
	chFinished := make(chan bool, POOL_SIZE)
	foundUrls := make(map[string]bool, POOL_SIZE)

	if depth <= 0 {
		return
	}
	depth--

	go func() {
		go_pool <- true
		defer func() {
			<-go_pool
		}()
		crawl(master_url, url, emails, chUrls, chFinished)
	}()

	// Subscribe to both channels
	for _b := false; ; {
		select {
		case new_url := <-chUrls:
			foundUrls[new_url] = true
		case <-chFinished:
			_b = true
			break
		}
		if _b == true {
			break
		}
	}

	if depth > 0 {
		_done := make(chan string)
		for _url, _ := range foundUrls {
			// fmt.Println(" - " + _url)
			go func() {
				go_pool <- true
				defer func() {
					<-go_pool
				}()
				go extract_emails(master_url, _url, _done, depth, emails)
			}()
		}

		for count := 0; count < len(foundUrls); count++ {
			<-_done
		}
	}

	close(chUrls)
	done <- url
}

func main() {
	//	resp, err := http.Get("http://manonthelam.com/")
	var go_pool = make(chan bool, POOL_SIZE)

	seedUrls := os.Args[1:]
	//seedUrls := []string{"http://manonthelam.com/"}
	done := make(chan string)
	emails := make(chan eRet, POOL_SIZE)

	// Kick off the crawl process (concurrently)
	for _, _url := range seedUrls {
		go func(master_url, url string) {
			go_pool <- true
			defer func() { <-go_pool }()
			extract_emails(url, url, done, DEPTH, emails)
		}(_url, _url)
	}

	emailMap := make(map[string]map[string]bool)
	for count := 0; count < len(seedUrls); {
		select {
		case <-done:
			count++

		case _email := <-emails:
			if _, ok := emailMap[_email.master_url]; ok == false {
				emailMap[_email.master_url] = make(map[string]bool)
			}

			if _, ok := emailMap[_email.master_url][_email.email]; ok == false {
				fmt.Println(_email.master_url, ":", _email.email)
			}
			emailMap[_email.master_url][_email.email] = true
		}
	}

	if len(emailMap) > 0 {
		fmt.Println("Final Summary: ")
		for key, val := range emailMap {
			fmt.Printf("%s : ", key)
			for _e, _ := range val {
				fmt.Printf("%s ", _e)
			}
			fmt.Println("")
		}
	}
}
