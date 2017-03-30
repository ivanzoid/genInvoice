package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

var (
	flagTemplateFilePath string
	flagConfigFilePath   string
	flagMode             string
)

const (
	kInvoiceKey        = "invoice"
	kDateKey           = "date"
	kGenInvoiceKey     = "gen_invoice"
	kGenDateCreatedKey = "gen_date_created"
	kGenDateDueKey     = "gen_date_due"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n\n")
	fmt.Fprintf(os.Stderr, "\t%s [options] <invoice.csv>\n", path.Base(os.Args[0]))
	fmt.Fprintf(os.Stderr, "\nOptions are:\n\n")
	flag.PrintDefaults()
}

func init() {
	flag.StringVar(&flagTemplateFilePath, "t", "", "Template file path")
	flag.StringVar(&flagConfigFilePath, "c", "", "Config file path")
	flag.StringVar(&flagMode, "m", "", "Mode: 'customer' or 'bank'")
}

// func readInvoice(filePath string) (values [][]string, err error) {
// 	file, err := os.Open(filePath)
// 	if err != nil {
// 		return
// 	}
// 	defer file.Close()

// 	reader := csv.NewReader(file)
// 	for {
// 		lineValues, err := reader.Read()
// 		if err == io.EOF {
// 			break
// 		} else if err != nil {
// 			fmt.Fprintf(os.Stderr, "Can't read CSV: %v\n", err)
// 			return nil, err
// 		}
// 		values = append(values, lineValues)
// 	}

// 	return
// }

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

func produceInvoiceTable(invoiceValues [][]interface{}) string {
	var invoiceTableBuffer bytes.Buffer

	for i, lineValues := range invoiceValues {
		isLastValue := i == len(invoiceValues)-1
		possibleLastString := ""
		if isLastValue {
			possibleLastString = " last"
		}
		fmt.Fprintf(&invoiceTableBuffer, "\t<tr class=\"item%v\">\n", possibleLastString)
		for _, value := range lineValues {
			fmt.Fprintf(&invoiceTableBuffer, "\t\t<td>%v</td>\n", value)
		}

		fmt.Fprintf(&invoiceTableBuffer, "\t</tr>\n")
		if !isLastValue {
			fmt.Fprintf(&invoiceTableBuffer, "\n")
		}
	}

	return invoiceTableBuffer.String()
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

	invoiceData, err := readInvoice(invoiceFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read invoice: %v\n", err)
		return
	}

	appendBrsToMultilineStringsInInvoiceData(invoiceData)

	invoiceTableAsArrayOfArrays, err := convertInvoiceTable(invoiceData[kInvoiceKey])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't parse invoice table: %v\n", err)
		return
	}

	invoiceTableAsString := produceInvoiceTable(invoiceTableAsArrayOfArrays)
	invoiceData[kGenInvoiceKey] = invoiceTableAsString
	invoiceData[kGenDateCreatedKey] = invoiceData[kDateKey]
	invoiceData[kGenDateDueKey] = invoiceData[kDateKey]

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
