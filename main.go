/* Build lambda and execute:
linux:

GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap main.go

win:
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -tags lambda.norpc -o bootstrap main.go
~\Go\Bin\build-lambda-zip.exe -o myFunction.zip bootstrap


update function:

aws lambda update-function-code --function-name myFunction \
--zip-file fileb://myFunction.zip
*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const yahooFinanceURL = "https://finance.yahoo.com/quote/"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
const bitSize = 64
const targetPerc = -19.0

const tableNameDDB = "QueryStocks"

type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type StockState struct {
	Ticker   string `json:"ticker"`
	LastDate string `json:"lastDate"`
}

func main() {
	_ = godotenv.Load()
	lambda.Start(HandleRequest)
}

func HandleRequest() (*string, error) {
	tickers := [7]string{"BQE.V", "HAYPP.ST", "TVK.TO", "SGN.WA", "CPH.TO", "CLPT", "SLYG.F"}
	client := &http.Client{}

	sessionDDB := session.Must(session.NewSession(&aws.Config{
		Region: aws.String("eu-central-1"), // Change to your desired region
	}))
	svc := dynamodb.New(sessionDDB)

	for _, ticker := range tickers {

		sent, err := checkStockNotifSent(svc, ticker)
		if err != nil || sent {
			continue
		}

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
		extractPrice(svc, doc, ticker)
		time.Sleep(2000)
	}

	message := fmt.Sprintf("Price job done!")
	return &message, nil
}

func extractPrice(svc *dynamodb.DynamoDB, doc *goquery.Document, ticker string) {
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

		checkTarget(svc, priceFloat, maxPriceFloat, ticker, currency)
	} else {
		log.Println("Could not find the stock price.")
	}
}

func checkTarget(svc *dynamodb.DynamoDB, currentPrice float64, maxPrice float64, ticker string, currency string) {
	currentPerc := ((currentPrice - maxPrice) / maxPrice) * 100
	if currentPerc <= targetPerc {
		message := fmt.Sprintf("Target reached on %s!\nCurrent price: %.2f %s\n54w high: %.2f %s\nDifference: %.2f%%\n", ticker, currentPrice, currency, maxPrice, currency, currentPerc)
		log.Println(message)

		tgBotToken := os.Getenv("TG_BOT_TOKEN")
		tgChatID := os.Getenv("TG_CHAT_ID")
		err := sendTgNotification(message, tgBotToken, tgChatID)
		if err != nil {
			log.Fatal("Error sending telegram notification:", err)
		}

		err = stockSent(svc, ticker)
		if err != nil {
			log.Fatal("Error creating or updating {} table on {}!", tableNameDDB, ticker)
		}
	}
}

func sendTgNotification(message string, botToken string, chatID string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	telegramMessage := TelegramMessage{
		ChatID: chatID,
		Text:   message,
	}

	body, err := json.Marshal(telegramMessage)
	if err != nil {
		return err
	}

	_, err = http.Post(url, "application/json", bytes.NewBuffer(body))
	return err
}

/*
Checks if stock notification has already been sent for today by querying dynamoDB table
*/
func checkStockNotifSent(svc *dynamodb.DynamoDB, ticker string) (bool, error) {
	today := time.Now().Format("2006-01-02")

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(tableNameDDB),
		Key: map[string]*dynamodb.AttributeValue{
			"ticker": {
				S: aws.String(ticker),
			},
		},
	})
	if err != nil {
		fmt.Println("Error getting item:", err)
		return false, err
	}

	var state StockState
	if result.Item != nil {
		err = dynamodbattribute.UnmarshalMap(result.Item, &state)
		if err != nil {
			fmt.Println("Failed to unmarshal:", err)
			return false, err
		}

		// Check if the notification has already been sent today
		if state.LastDate == today {
			fmt.Println("Notification already sent for {} today!", ticker)
			return true, nil
		} else {
			return false, nil
		}
	}
	return false, nil // non-existent ticker in dynamo
}

/*
Create or update ticker on dynamo with today's date signaling notification sent
*/
func stockSent(svc *dynamodb.DynamoDB, ticker string) error {
	today := time.Now().Format("2006-01-02")

	var state StockState
	state = StockState{
		Ticker:   ticker,
		LastDate: today,
	}

	item, err := dynamodbattribute.MarshalMap(state)
	if err != nil {
		fmt.Println("Failed to marshal:", err)
		return err
	}

	_, err = svc.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableNameDDB),
		Item:      item,
	})
	if err != nil {
		fmt.Println("Failed to put item:", err)
		return err
	}

	return nil
}
