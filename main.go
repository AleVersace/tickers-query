package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/robfig/cron/v3"
	"log"
	"net/http"
	"os"
	"time"
)

const yahooFinanceURL = "https://finance.yahoo.com/quote/"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"

func main() {
	c := cron.New()

	_, err := c.AddFunc("0,15,30,45 * * * *", func() {
		priceJob()
	})
	if err != nil {
		os.Exit(1)
	}

	c.Start()

	select {}
}

func priceJob() {
	tickers := [2]string{"BQE.V", "HAYPP.ST"}
	client := &http.Client{}
	for _, ticker := range tickers {
		url := yahooFinanceURL + ticker + "/"
		// request
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Fatal("Error creating request:", err)
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("Error making request:", err)
		}
		// decode request
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			log.Fatal("Error parsing HTML:", err)
		}
		err = resp.Body.Close()
		if err != nil {
			log.Fatal("Error during body Close: ", err)
		}
		extractPrice(doc, ticker)
		time.Sleep(2000)
	}
}

func extractPrice(doc *goquery.Document, ticker string) {
	// The stock price on the Yahoo Finance page is within a <fin-streamer> tag with a specific class
	priceSelector := fmt.Sprintf("fin-streamer[data-field='regularMarketPrice'][data-symbol='%s']", ticker)
	currencySelector := ".exchange > :last-child"

	price := doc.Find(priceSelector).Text()
	currency := doc.Find(currencySelector).Text()
	if price != "" && currency != "" {
		log.Printf("The current price of %s is: %s %s\n", ticker, price, currency)
	} else {
		log.Println("Could not find the stock price.")
	}
}
