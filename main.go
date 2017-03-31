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

	for key, value := range invoiceUntilteredData {
		if keyAsString, ok := key.(string); ok {
			invoiceData[keyAsString] = value
		} else {
			fmt.Fprintf(os.Stderr, "Key '%v' is not string - ignoring.\n", key)
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
	if len(invoiceValues) == 0 {
		return
	}
	firstLine := invoiceValues[0]

	if len(firstLine) == 0 {
		return nil, 0, errors.New("Invoice table header is empty")
	}

	amountColumnIndex := -1
	hoursColumnIndex := -1
	for i, value := range firstLine {
		valueAsString, ok := value.(string)
		if ok {
			if strings.Contains(strings.ToLower(valueAsString), "amount") {
				amountColumnIndex = i
			} else if strings.Contains(strings.ToLower(valueAsString), "hours") {
				hoursColumnIndex = i
			}
		}
	}

	if amountColumnIndex == -1 {
		fmt.Fprintf(os.Stderr, "Can't find column with \"Amount\", assuming it is last column.")
		amountColumnIndex = len(firstLine) - 1
	}

	if hoursColumnIndex == -1 {
		fmt.Fprintf(os.Stderr, "Can't find column with \"Hours\", will omit hours in total line.")
	}

	total = 0.0
	hoursTotal := 0.0

	for i := 1; i < len(invoiceValues); i++ {
		line := invoiceValues[i]
		if amountColumnIndex >= len(line) {
			fmt.Fprintf(os.Stderr, "Missing amount at line %v.\n", i)
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

func makeTotalLineInUsd(total float64, totalInUsd float64, columnCount int) (totalLine []interface{}) {
	for i := 0; i < columnCount; i++ {
		if i == 0 {
			totalLine = append(totalLine, fmt.Sprintf("Total in USD using USD/AUD rate = %.6f", total/totalInUsd))
		} else if i == columnCount-1 {
			totalLine = append(totalLine, fmt.Sprintf("%v", totalInUsd))
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

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
		return
	}
	invoiceFilePath := flag.Arg(0)

	configData, _ := readInvoice(flagConfigFilePath)

	invoiceData, err := readInvoice(invoiceFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read invoice: %v\n", err)
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

	invoiceTableAsArrayOfArrays, err := convertInvoiceTable(invoiceData[kInvoiceKey])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't parse invoice table: %v\n", err)
		return
	}

	invoiceTotalLine, total, err := calculateInvoiceTotal(invoiceTableAsArrayOfArrays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't calculate total: %v\n", err)
	} else {
		invoiceTableAsArrayOfArrays = append(invoiceTableAsArrayOfArrays, invoiceTotalLine)
	}

	totalLineCount := 1

	if invoiceData[kReceivedUsdKey] != nil {
		totalInUsd := interfaceToFloat(invoiceData[kReceivedUsdKey])
		if totalInUsd != 0 {
			totalLineInUsd := makeTotalLineInUsd(total, totalInUsd, len(invoiceTotalLine))
			invoiceTableAsArrayOfArrays = append(invoiceTableAsArrayOfArrays, totalLineInUsd)
			totalLineCount++
		}
	}

	invoiceTableAsString := produceInvoiceTable(invoiceTableAsArrayOfArrays, totalLineCount)
	invoiceData[kGenInvoiceKey] = invoiceTableAsString

	if invoiceData[kDateKey] != nil {
		dateValue := invoiceData[kDateKey]
		if dateAsString, ok := dateValue.(string); ok {
			date, err := time.Parse("2006-01-02", dateAsString)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Can't parse '%v': %v\n", kDateKey, err)
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
		fmt.Fprintf(os.Stderr, "Can't create template: %v\n", err)
		return
	}

	err = tmpl.Execute(os.Stdout, invoiceData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't render template: %v\n", err)
		return
	}
}
