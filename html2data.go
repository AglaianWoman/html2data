/*
Package html2data - extract data from HTML via CSS selectors

Install package and command line utility:

	go get -u github.com/msoap/html2data/cmd/html2data

Install package only:

	go get -u github.com/msoap/html2data

Allowed pseudo-selectors:

`:attr(attr_name)` - for getting attributes instead text

`:html` - for getting HTML instead text

`:get(N)` - get n-th element from list

Command line utility:

    html2data URL "css selector"
    html2data file.html "css selector"
    cat file.html | html2data "css selector"

*/
package html2data

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// docOrSelection - for exec .Find
type docOrSelection interface {
	Find(string) *goquery.Selection
}

// Doc - html document for parse
type Doc struct {
	doc docOrSelection
	Err error
}

// CSSSelector - selector with settings
type CSSSelector struct {
	selector string
	attrName string
	getHTML  bool
	getNth   int
}

// getDataFromDocOrSelection - extract data by CSS-selectors from goquery.Selection or goquery.Doc
func (doc Doc) getDataFromDocOrSelection(docOrSelection docOrSelection, selectors map[string]string) (result map[string][]string, err error) {
	if doc.Err != nil {
		return result, fmt.Errorf("parse document error: %s", doc.Err)
	}

	result = map[string][]string{}
	for name, selectorRaw := range selectors {
		selector := parseSelector(selectorRaw)

		texts := []string{}
		docOrSelection.Find(selector.selector).Each(func(i int, selection *goquery.Selection) {
			if selector.getNth > 0 && selector.getNth != i+1 {
				return
			}

			if selector.attrName != "" {
				texts = append(texts, selection.AttrOr(selector.attrName, ""))
			} else if selector.getHTML {
				HTML, err := selection.Html()
				if err != nil {
					return
				}
				texts = append(texts, HTML)
			} else {
				texts = append(texts, selection.Text())
			}
		})
		result[name] = texts
	}

	return result, err
}

// parseSelector - parse pseudo-selectors:
// :attr(href) - for getting attribute instead text node
func parseSelector(inputSelector string) (outSelector CSSSelector) {
	htmlAttrRe := regexp.MustCompile(`^\s*(\w+)\s*(?:\(\s*(\w+)\s*\))?\s*$`)

	parts := strings.Split(inputSelector, ":")
	outSelector.selector, parts = parts[0], parts[1:]
	for _, part := range parts {
		reParts := htmlAttrRe.FindStringSubmatch(part)
		switch {
		case len(reParts) == 3 && reParts[1] == "attr":
			outSelector.attrName = reParts[2]
		case len(reParts) == 3 && reParts[1] == "html":
			outSelector.getHTML = true
		case len(reParts) == 3 && reParts[1] == "get":
			outSelector.getNth, _ = strconv.Atoi(reParts[2])
		default:
			outSelector.selector += ":" + part
		}
	}

	return outSelector
}

// GetData - extract data by CSS-selectors
//  texts, err := doc.GetData(map[string]string{"h1": "h1"})
func (doc Doc) GetData(selectors map[string]string) (result map[string][]string, err error) {
	result, err = doc.getDataFromDocOrSelection(doc.doc, selectors)
	return result, err
}

// GetDataNested - extract nested data by CSS-selectors from another CSS-selector
//  texts, err := doc.GetDataNested("CSS.selector", map[string]string{"h1": "h1"}) - get h1 from CSS.selector
func (doc Doc) GetDataNested(selectorRaw string, nestedSelectors map[string]string) (result []map[string][]string, err error) {
	selector := parseSelector(selectorRaw)
	doc.doc.Find(selector.selector).Each(func(i int, selection *goquery.Selection) {
		if selector.getNth > 0 && selector.getNth != i+1 {
			return
		}

		nestedResult, nestedErr := doc.getDataFromDocOrSelection(selection, nestedSelectors)
		if nestedErr != nil {
			err = nestedErr
		}

		result = append(result, nestedResult)
	})
	return result, err
}

// GetDataSingle - extract data by one CSS-selector
//  title, err := doc.GetDataSingle("title")
func (doc Doc) GetDataSingle(selector string) (result string, err error) {
	texts, err := doc.GetData(map[string]string{"single": selector})
	if err != nil {
		return result, err
	}

	if textOne, ok := texts["single"]; ok && len(textOne) > 0 {
		result = textOne[0]
	}

	return result, err
}

// FromReader - get doc from io.Reader
func FromReader(reader io.Reader) Doc {
	doc, err := goquery.NewDocumentFromReader(reader)
	return Doc{doc, err}
}

// FromFile - get doc from file
func FromFile(fileName string) Doc {
	fileReader, err := os.Open(fileName)
	if err != nil {
		return Doc{Err: err}
	}
	defer fileReader.Close()

	return FromReader(fileReader)
}

// Cfg - config for FromURL()
type Cfg struct {
	UA      string // custom user-agent
	TimeOut int    // timeout in seconds
}

// FromURL - get doc from URL
//
// FromURL("https://url")
// FromURL("https://url", Cfg{UA: "Custom UA 1.0", TimeOut: 10})
func FromURL(URL string, config ...Cfg) Doc {
	ua, timeout := "", 0
	if len(config) > 0 {
		ua = config[0].UA
		timeout = config[0].TimeOut
	}
	httpResponse, err := getHTMLPage(URL, ua, timeout)
	if err != nil {
		return Doc{Err: err}
	}

	return FromReader(httpResponse.Body)
}

// getHTMLPage - get html by http(s) as http.Response
func getHTMLPage(url string, ua string, timeout int) (response *http.Response, err error) {
	cookie, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     cookie,
		Timeout: time.Duration(time.Duration(timeout) * time.Second),
	}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return response, err
	}

	if ua != "" {
		request.Header.Set("User-Agent", ua)
	}

	response, err = client.Do(request)
	if err != nil {
		return response, err
	}

	return response, err
}
