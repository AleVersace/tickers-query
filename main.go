package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const yahooFinanceURL = "https://finance.yahoo.com/quote/"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
const bitSize = 64
const targetPerc = -19.0

func main() {
	priceJob()
}

func priceJob() {
	tickers := [3]string{"BQE.V", "HAYPP.ST", "MOB.ST"}
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
	highLowPriceSelector := fmt.Sprintf("fin-streamer[data-field='fiftyTwoWeekRange'][data-symbol='%s']", ticker)
	currencySelector := ".exchange > :last-child"

	price := doc.Find(priceSelector).Text()
	highLowPrice := doc.Find(highLowPriceSelector).Text()

	parts := strings.Split(highLowPrice, " - ")
	maxPrice := strings.TrimSpace(parts[1])

	currency := doc.Find(currencySelector).Text()
	if price != "" && currency != "" && maxPrice != "" {
		log.Printf("The current price of %s is: %s %s\n", ticker, price, currency)
		log.Printf("52 weeks high of %s was: %s\n", ticker, maxPrice)

		priceFloat, err := strconv.ParseFloat(price, bitSize)
		if err != nil {
			log.Fatal("Error converting price to float: ", err)
			return
		}
		maxPriceFloat, err := strconv.ParseFloat(maxPrice, bitSize)
		if err != nil {
			log.Fatal("Error converting price to float: ", err)
			return
		}

		checkTarget(priceFloat, maxPriceFloat)
	} else {
		log.Println("Could not find the stock price.")
	}
}

func checkTarget(currentPrice float64, maxPrice float64) {
	currentPerc := ((currentPrice - maxPrice) / maxPrice) * 100
	if currentPerc <= targetPerc {
		log.Printf("Target reached! %.2f", currentPerc)
	}
}
