package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
)

var (
	flagTemplateFilePath string
	flagConfigFilePath   string
	flagMode             string
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

func readInvoice(filePath string) (values [][]string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	for {
		lineValues, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Can't read CSV: %v\n", err)
			return nil, err
		}
		values = append(values, lineValues)
	}

	return
}

func readTemplate(filePath string) (template string, err error) {
	buffer, err := ioutil.ReadFile(filePath)
	if err != nil {
		return
	}

	template = string(buffer)
	return
}

func produceInvoiceTable(invoiceValues [][]string) string {
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

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
		return
	}
	invoiceFilePath := flag.Arg(0)

	invoiceValues, err := readInvoice(invoiceFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read invoice: %v\n", err)
		return
	}

	// template, err := readTemplate(flagTemplateFilePath)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Can't read template file: %v\n", err)
	// 	return
	// }

	fmt.Println(produceInvoiceTable(invoiceValues))
}
