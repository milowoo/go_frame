package new_frame

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"new_frame/src/lgames"
	"time"
)

func HttpPost(remoteUrl string, params map[string]interface{}, result interface{}, log *lgames.Logger) (jsonResult map[string]interface{}, err error) {
	queryStrings := []string{}
	if len(params) > 0 {
		for k, v := range params {
			queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, url.QueryEscape(fmt.Sprintf("%v", v))))
		}
	}

	request, _ := http.NewRequest(http.MethodPost, remoteUrl, bytes.NewBufferString(strings.Join(queryStrings, "&")))
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	client := http.DefaultClient
	client.Timeout = 10 * time.Second
	if strings.HasPrefix(remoteUrl, "https:") {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	response, err := client.Do(request)
	if err != nil {
		log.Error("%v", err)
		return
	}
	if response == nil {
		err = errors.New(fmt.Sprintf("http response is null: %s", err.Error()))
		return
	}

	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(response.Body)
	body := string(buf.Bytes())
	log.Debug("stausCode=%d err=%v body=%v readErr=%v", response.StatusCode, err, body, readErr)

	if response.StatusCode != 200 {
		err = errors.New(fmt.Sprintf("http status code:%d", response.StatusCode))
		return
	}

	var f interface{}
	err = json.Unmarshal(buf.Bytes(), &f)
	if err != nil {
		return
	}
	jsonResult = f.(map[string]interface{})
	if result != nil {
		err = json.Unmarshal(buf.Bytes(), result)
	}
	return
}
