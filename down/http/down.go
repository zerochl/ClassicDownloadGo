package http

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"ClassicDownloadGo/down/http/entity/response"
	"ClassicDownloadGo/down/http/entity/request"
	"github.com/gogather/com/log"
	"ClassicDownloadGo/down/http/split"
)

var (
	downloadPath string
)

type Download interface {
	Init(initRequest *request.InitRequest)
	Resolve(request *http.Request) (*http.Response, error)
	Down(request *http.Request) error
}

func Init(initRequest *request.InitRequest)  {
	downloadPath = initRequest.DownloadPath
	split.Init(initRequest.MinSplitBurst)
}

// Resolve return the file response to be downloaded
func Resolve(request *request.Request) (*response.Response, error) {
	httpRequest, err := BuildHTTPRequest(request)
	if err != nil {
		return nil, err
	}
	// Use "Range" header to resolve
	httpRequest.Header.Add("Range", "bytes=0-0")
	httpClient := BuildHTTPClient()
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != 200 && httpResponse.StatusCode != 206 {
		return nil, fmt.Errorf("response status error:%d", httpResponse.StatusCode)
	}
	ret := &response.Response{}
	// Get file name by "Content-Disposition"
	contentDisposition := httpResponse.Header.Get("Content-Disposition")
	if contentDisposition != "" {
		_, params, _ := mime.ParseMediaType(contentDisposition)
		filename := params["filename"]
		if filename != "" {
			ret.Name = filename
		}
	}
	// Get file name by URL
	if ret.Name == "" {
		parse, err := url.Parse(httpRequest.URL.String())
		if err == nil {
			// e.g. /files/test.txt => test.txt
			ret.Name = subLastSlash(parse.Path)
		}
	}
	// Unknow file name
	if ret.Name == "" {
		ret.Name = "unknow"
	}
	// Is support range
	ret.Range = httpResponse.StatusCode == 206
	// Get file size
	if ret.Range {
		contentRange := httpResponse.Header.Get("Content-Range")
		if contentRange != "" {
			// e.g. bytes 0-1000/1001 => 1001
			total := subLastSlash(contentRange)
			if total != "" && total != "*" {
				parse, err := strconv.ParseInt(total, 10, 64)
				if err != nil {
					return nil, err
				}
				ret.Size = parse
			}
		}
	} else {
		contentLength := httpResponse.Header.Get("Content-Length")
		if contentLength != "" {
			ret.Size, _ = strconv.ParseInt(contentLength, 10, 64)
		}
	}
	return ret, nil
}

// Down
func Down(request *request.Request) error {
	response, err := Resolve(request)
	if err != nil {
		return err
	}
	// allocate file
	file, err := os.Create(downloadPath + response.Name)
	if err != nil {
		return err
	}
	defer file.Close()
	log.Println("size:", response.Size, ";rang:", response.Range)
	if err := file.Truncate(response.Size); err != nil {
		return err
	}
	// support range
	if response.Range {
		splitCount, splitSize := split.CaculateBurst(response.Size)
		var (
			waitGroup = &sync.WaitGroup{}
			fileLock  = &sync.Mutex{}
		)
		waitGroup.Add(splitCount)
		for i := 0; i < splitCount; i++ {
			start := int64(i) * splitSize
			end := start + splitSize
			if i == splitCount -1 {
				end = response.Size
			}
			go downChunk(request, file, start, end-1, waitGroup, fileLock)
		}
		waitGroup.Wait()
	} else {
		downChunk(request, file, 0, response.Size, nil, nil)
	}
	return nil
}

func subLastSlash(str string) string {
	index := strings.LastIndex(str, "/")
	if index != -1 {
		return str[index+1:]
	}
	return ""
}

func BuildHTTPRequest(request *request.Request) (*http.Request, error) {
	// Build request
	httpRequest, err := http.NewRequest(strings.ToUpper(request.Method), request.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range request.Header {
		httpRequest.Header.Add(k, v)
	}
	return httpRequest, nil
}

func BuildHTTPClient() *http.Client {
	// Cookie handle
	jar, _ := cookiejar.New(nil)

	return &http.Client{Jar: jar}
}

func downChunk(request *request.Request, file *os.File, start int64, end int64, waitGroup *sync.WaitGroup, fileLock *sync.Mutex) {
	if waitGroup != nil {
		defer waitGroup.Done()
	}
	httpRequest, _ := BuildHTTPRequest(request)
	httpRequest.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	fmt.Printf("down %d-%d\n", start, end)
	httpClient := BuildHTTPClient()
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer httpResponse.Body.Close()
	buf := make([]byte, 8192)
	writeIndex := int64(start)
	for {
		n, err := httpResponse.Body.Read(buf)
		if n > 0 {
			fileLock.Lock()
			writeSize, err := file.WriteAt(buf[0:n], writeIndex)
			fileLock.Unlock()
			if err != nil {
				fmt.Println(err)
				return
			}
			writeIndex += int64(writeSize)
		}
		if err != nil {
			if err != io.EOF {
				fmt.Println(err)
				return
			}
			break
		}
	}
}
