package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

var (
	flagTemplateFilePath string
	flagConfigFilePath   string
	flagMode             string
	flagGenerateForMonth string
)

const (
	kConfigDir         = ".genInvoice"
	kInvoiceKey        = "invoice"
	kDateKey           = "date"
	kReceivedUsdKey    = "received_usd"
	kHourlyRateKey     = "hourly_rate"
	kCurrencyKey       = "currency"
	kGenInvoiceKey     = "gen_invoice"
	kGenDateCreatedKey = "gen_date_created"
	kGenDateDueKey     = "gen_date_due"
	kAmount            = "amount"
	kHours             = "hours"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n\n")
	fmt.Fprintf(os.Stderr, "\t%s [options] <invoice.yaml>\n", path.Base(os.Args[0]))
	fmt.Fprintf(os.Stderr, "\nOptions are:\n\n")
	flag.PrintDefaults()
}

func init() {
	defaultTemplateFilePath := ""
	defaultConfigFilePath := ""
	usr, err := user.Current()
	if err == nil {
		defaultTemplateFilePath = filepath.Join(usr.HomeDir, ".genInvoice", "Invoice.html.tmpl")
		defaultConfigFilePath = filepath.Join(usr.HomeDir, ".genInvoice", "config.yaml")
	}
	flag.StringVar(&flagTemplateFilePath, "t", defaultTemplateFilePath, "Template file path")
	flag.StringVar(&flagConfigFilePath, "c", defaultConfigFilePath, "Config file path")
	flag.StringVar(&flagGenerateForMonth, "g", "", "If given, sample invoice (in YAML) for specified month index will be generated")
}

func log(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func readInvoice(filePath string) (invoiceData map[string]interface{}, err error) {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return
	}

	var data interface{}
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return
	}

	invoiceUntilteredData, ok := data.(map[interface{}]interface{})
	if !ok {
		return nil, errors.New("Yaml root object is not dictionary")
	}

	invoiceData = make(map[string]interface{})

	log("Using data from %v\n", filePath)

	for key, value := range invoiceUntilteredData {
		if keyAsString, ok := key.(string); ok {
			invoiceData[keyAsString] = value
		} else {
			log("Key '%v' is not string - ignoring.\n", key)
		}
	}

	return invoiceData, nil
}

func produceInvoiceTable(invoiceValues [][]interface{}, totalLineCount int) string {
	var invoiceTableBuffer bytes.Buffer

	for i, lineValues := range invoiceValues {
		class := "item"
		if i == 0 {
			class = "heading"
		} else if i >= (len(invoiceValues) - totalLineCount) {
			class = "total"
		}
		fmt.Fprintf(&invoiceTableBuffer, "\t<tr class=\"%v\">\n", class)
		for _, value := range lineValues {
			fmt.Fprintf(&invoiceTableBuffer, "\t\t<td>%v</td>\n", value)
		}

		fmt.Fprintf(&invoiceTableBuffer, "\t</tr>\n")
	}

	return invoiceTableBuffer.String()
}

func calculateInvoiceTotal(invoiceValues [][]interface{}) (invoiceTotalLine []interface{}, total float64, err error) {
	amountColumnIndex := findAmountColumnIndex(invoiceValues)
	hoursColumnIndex := findHoursColumnIndex(invoiceValues)

	if amountColumnIndex == -1 {
		err = errors.New(fmt.Sprintf("Can't find column with \"%v\".", kAmount))
		return
	}

	if hoursColumnIndex == -1 {
		log("Can't find column with \"%v\", will omit hours in total line.", kHours)
	}

	total = 0.0
	hoursTotal := 0.0

	for i := 1; i < len(invoiceValues); i++ {
		line := invoiceValues[i]
		if amountColumnIndex >= len(line) {
			log("Missing amount at line %v.\n", i)
		} else {
			amount := interfaceToFloat(line[amountColumnIndex])
			total += amount
		}
		if hoursColumnIndex < len(line) {
			hours := interfaceToFloat(line[hoursColumnIndex])
			hoursTotal += hours
		}
	}

	for i := 0; i <= amountColumnIndex; i++ {
		if i == 0 {
			invoiceTotalLine = append(invoiceTotalLine, "Total")
		} else if i == amountColumnIndex {
			invoiceTotalLine = append(invoiceTotalLine, fmt.Sprintf("%v", total))
		} else if i == hoursColumnIndex {
			invoiceTotalLine = append(invoiceTotalLine, fmt.Sprintf("%v", hoursTotal))
		} else {
			invoiceTotalLine = append(invoiceTotalLine, "")
		}
	}

	return
}

func appendOrFillAmountIfNeeded(invoiceValues [][]interface{}, hourlyRate float64) {
	if len(invoiceValues) == 0 {
		return
	}
	firstLine := invoiceValues[0]
	amountColumnIndex := findAmountColumnIndex(invoiceValues)
	hoursColumnIndex := findHoursColumnIndex(invoiceValues)

	if amountColumnIndex == -1 {
		log("Can't find column with \"%v\", will append to the right column.", kAmount)
		amountColumnIndex = len(firstLine)
		invoiceValues[0] = append(invoiceValues[0], "Amount")
	}

	if hoursColumnIndex == -1 {
		if amountColumnIndex == -1 {
			log("Can't find columns with \"%v\" or \"%v\".", kAmount, kHours)
			return
		}
	}

	for i := 1; i < len(invoiceValues); i++ {
		line := invoiceValues[i]

		if hoursColumnIndex >= len(line) {
			log("Missing hours column at line %v.\n", i)
			continue
		}

		hours := interfaceToFloat(line[hoursColumnIndex])
		amount := hours * hourlyRate
		existingAmount := 0.0

		if amountColumnIndex < len(line) {
			existingAmount = interfaceToFloat(line[amountColumnIndex])
			if existingAmount == 0.0 {
				invoiceValues[i][amountColumnIndex] = amount
			}
		} else {
			invoiceValues[i] = append(invoiceValues[i], amount)
		}
	}
}

func interfaceToFloat(value interface{}) float64 {
	switch typedValue := value.(type) {
	case float64:
		return typedValue
	case float32:
		return float64(typedValue)
	case int:
		return float64(typedValue)
	case uint:
		return float64(typedValue)
	case string:
		floatValue, _ := strconv.ParseFloat(typedValue, 10)
		return floatValue
	default:
		return 0.0
	}
}

func interfaceToString(value interface{}) string {
	switch typedValue := value.(type) {
	case string:
		return typedValue
	default:
		return ""
	}
}

func makeTotalLineInUsd(total float64, totalInUsd float64, columnCount int) (totalLine []interface{}) {
	for i := 0; i < columnCount; i++ {
		if i == 0 {
			totalLine = append(totalLine, fmt.Sprintf("Total in $USD (with USD/AUD rate = %.4f)", total/totalInUsd))
		} else if i == columnCount-1 {
			totalLine = append(totalLine, fmt.Sprintf("$USD %v", totalInUsd))
		} else {
			totalLine = append(totalLine, "")
		}
	}

	return
}

func convertInvoiceTable(invoiceTable interface{}) (invoiceValues [][]interface{}, err error) {
	if invoiceTable == nil {
		return nil, errors.New("missing table")
	}

	invoiceTableAsArray, ok := invoiceTable.([]interface{})
	if !ok {
		return nil, errors.New(fmt.Sprintf("type %T is not array", invoiceTable))
	}

	for i, value := range invoiceTableAsArray {
		valueAsArray, ok := value.([]interface{})
		if !ok {
			return nil, errors.New(fmt.Sprintf("object at index %v with type %T is not array", i, invoiceTable))
		}
		invoiceValues = append(invoiceValues, valueAsArray)
	}

	return
}

func findAmountColumnIndex(invoiceValues [][]interface{}) int {
	return findColumnIndex(invoiceValues, kAmount)
}

func findHoursColumnIndex(invoiceValues [][]interface{}) int {
	return findColumnIndex(invoiceValues, kHours)
}

func findColumnIndex(invoiceValues [][]interface{}, columnSubString string) (columnIndex int) {
	columnIndex = -1
	if len(invoiceValues) == 0 {
		return
	}

	firstLine := invoiceValues[0]

	for i, value := range firstLine {
		valueAsString, ok := value.(string)
		if ok {
			if strings.Contains(strings.ToLower(valueAsString), columnSubString) {
				columnIndex = i
			}
		}
	}
	return columnIndex
}

func addCurrencyToAmountColumn(invoiceValues [][]interface{}, currency string) {
	amountColumnIndex := findAmountColumnIndex(invoiceValues)

	if amountColumnIndex == -1 {
		log("Can't find column with \"%v\".", kAmount)
		return
	}

	for i := 1; i < len(invoiceValues); i++ {
		line := invoiceValues[i]

		if amountColumnIndex >= len(line) {
			log("Missing \"%v\" at line %v.\n", kAmount, i)
			continue
		}
		amount := interfaceToFloat(line[amountColumnIndex])
		invoiceValues[i][amountColumnIndex] = fmt.Sprintf("%v %v", currency, amount)
	}
}

func appendBrsToMultilineStringsInInvoiceData(invoiceData map[string]interface{}) {
	for key, value := range invoiceData {
		if valueAsString, ok := value.(string); ok {
			lines := strings.Split(valueAsString, "\n")
			if len(lines) >= 2 {
				for i, line := range lines {
					lines[i] = fmt.Sprintf("%v<br>", line)
				}
				linesAsString := strings.Join(lines, "\n")
				invoiceData[key] = linesAsString
			}
		}
	}
}

func isWorkDay(t time.Time) bool {
	return t.Weekday() >= time.Monday && t.Weekday() <= time.Friday
}

func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func generateSampleInvoiceForMonth(monthOffset int) {
	fmt.Printf("date: %v\n", time.Now().Format("2006-01-02"))
	fmt.Printf("invoice:\n")
	fmt.Printf("  - [Dates,Hours worked,Amount]\n")

	dayMonthAgo := time.Now().AddDate(0, -monthOffset, 0)
	firstDayInMonth := dayMonthAgo.AddDate(0, 0, -(dayMonthAgo.Day() - 1))
	totalDays := daysInMonth(firstDayInMonth)
	weekStartDay := -1
	weekEndDay := -1
	for day := 0; day <= totalDays; day++ {
		t := firstDayInMonth.AddDate(0, 0, day)
		if isWorkDay(t) {
			if weekStartDay == -1 && day != totalDays {
				weekStartDay = day
			}
		} else {
			if weekStartDay != -1 {
				weekEndDay = day - 1
			}
		}
		if weekStartDay != -1 && weekEndDay != -1 {
			fmt.Printf("  - [%v %v-%v,%v]\n", firstDayInMonth.Format("January"), weekStartDay+1, weekEndDay+1, (weekEndDay-weekStartDay+1)*8)
			weekStartDay = -1
			weekEndDay = -1
		}
	}
}

func main() {
	flag.Parse()

	if len(flagGenerateForMonth) != 0 {
		monthOffset, err := strconv.ParseInt(flagGenerateForMonth, 10, 0)
		if err != nil {
			log("Can't parse month: %v\n", err)
		} else {
			generateSampleInvoiceForMonth(int(monthOffset))
		}
		return
	}

	if flag.NArg() < 1 {
		usage()
		return
	}

	invoiceFilePath := flag.Arg(0)

	configData, _ := readInvoice(flagConfigFilePath)

	invoiceData, err := readInvoice(invoiceFilePath)
	if err != nil {
		log("Can't read invoice: %v\n", err)
		return
	}

	if configData != nil {
		for key, value := range configData {
			if invoiceData[key] == nil {
				invoiceData[key] = value
			}
		}
	}

	appendBrsToMultilineStringsInInvoiceData(invoiceData)

	invoiceTable, err := convertInvoiceTable(invoiceData[kInvoiceKey])
	if err != nil {
		log("Can't parse invoice table: %v\n", err)
		return
	}

	hourlyRate := interfaceToFloat(invoiceData[kHourlyRateKey])

	appendOrFillAmountIfNeeded(invoiceTable, hourlyRate)

	invoiceTotalLine, total, err := calculateInvoiceTotal(invoiceTable)
	if err != nil {
		log("Can't calculate total: %v\n", err)
	} else {
		invoiceTable = append(invoiceTable, invoiceTotalLine)
	}

	currency := interfaceToString(invoiceData[kCurrencyKey])

	addCurrencyToAmountColumn(invoiceTable, currency)

	totalLineCount := 1

	if invoiceData[kReceivedUsdKey] != nil {
		totalInUsd := interfaceToFloat(invoiceData[kReceivedUsdKey])
		if totalInUsd != 0 {
			totalLineInUsd := makeTotalLineInUsd(total, totalInUsd, len(invoiceTotalLine))
			invoiceTable = append(invoiceTable, totalLineInUsd)
			totalLineCount++
		}
	}

	invoiceTableAsString := produceInvoiceTable(invoiceTable, totalLineCount)
	invoiceData[kGenInvoiceKey] = invoiceTableAsString

	if invoiceData[kDateKey] != nil {
		dateValue := invoiceData[kDateKey]
		if dateAsString, ok := dateValue.(string); ok {
			date, err := time.Parse("2006-01-02", dateAsString)
			if err != nil {
				log("Can't parse '%v': %v\n", kDateKey, err)
			} else {
				invoiceData[kDateKey] = date.Format("20060102")
				invoiceData[kGenDateCreatedKey] = date.Format("2 January 2006")
				dueDate := date.AddDate(0, 0, 14)
				invoiceData[kGenDateDueKey] = dueDate.Format("2 January 2006")
			}
		}
	}

	tmpl, err := template.ParseFiles(flagTemplateFilePath)
	if err != nil {
		log("Can't create template: %v\n", err)
		return
	}

	log("Using template %v\n", flagTemplateFilePath)

	err = tmpl.Execute(os.Stdout, invoiceData)
	if err != nil {
		log("Can't render template: %v\n", err)
		return
	}

	log("Done.\n")
}
