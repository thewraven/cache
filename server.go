package main

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Status = "Status"

func serve(info serverInfo) {
	server := newServer(info)
	httpServer := http.Server{
		Handler: server,
		Addr:    info.LocalPort,
	}
	go func() {
		httpServer.ListenAndServe()
	}()
}

type requestInfo struct {
	requestFile     string
	requestHeaders  string
	responseFile    string
	responseHeaders string
}

type server struct {
	RemoteAddress string
	CacheFolder   string
	Timeout       time.Duration
	mapTimeouts   map[string]time.Time
	l             logger
}

type logger interface {
	Println(...interface{})
}

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpsProto = "https://"
	httpProto  = "http://"
)

func newServer(info serverInfo) *server {
	log.Println(info)
	pf := info.Name
	if pf == "" {
		pf = info.CachePath
	}
	pf += ": "
	l := log.New(os.Stdout, pf, log.Flags())
	server := server{
		RemoteAddress: info.RemoteAddress,
		CacheFolder:   info.CachePath,
		Timeout:       time.Duration(info.Timeout) * time.Minute,
		mapTimeouts:   make(map[string]time.Time),
		l:             l,
	}
	os.MkdirAll(server.CacheFolder, os.ModeDir|0755)
	//check for existing files in cache
	visitor := func(_ string, info os.FileInfo, _ error) error {
		//ignore request files
		if !strings.Contains(info.Name(), ".req") {
			//save hash of content and its modification time
			server.mapTimeouts[info.Name()] = info.ModTime()
		}
		return nil
	}
	filepath.Walk(server.CacheFolder, visitor)
	return &server
}

func (s *server) hashRequest(r *http.Request) (bytes.Buffer, int, string) {
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	//content + URI
	r.Body.Close()
	contentSize := buf.Len()
	buf.WriteString(r.RequestURI)
	hash := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))
	s.l.Println(hash)
	return buf, contentSize, hash
}

func (s *server) request(uri string, buf bytes.Buffer, sz int, r *http.Request, client http.Client) (*http.Response, error) {
	contentBuf := bytes.NewBuffer(buf.Bytes())
	contentBuf.Truncate(sz)
	s.l.Println(r.Method + " request to " + contentBuf.String())
	request, err := http.NewRequest(r.Method, uri, contentBuf)
	if err != nil {
		s.l.Println(r.Method + " request to " + uri + " failed: " + err.Error())
		return nil, err
	}
	s.copyHeaders(request, r)
	resp, err := client.Do(request)
	if err != nil {
		s.l.Println(r.Method + " request to " + uri + " failed: " + err.Error())
		return nil, err
	}
	return resp, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//hash the request info
	buf, contentSize, hash := s.hashRequest(r)
	//check if the content is not cached
	cacheTime, ok := s.mapTimeouts[hash]
	f := requestInfo{
		requestFile:    filepath.Join(s.CacheFolder, hash+".req"),
		requestHeaders: filepath.Join(s.CacheFolder, hash+".req.headers.json"),

		responseHeaders: filepath.Join(s.CacheFolder, hash+".res.headers.json"),
		responseFile:    filepath.Join(s.CacheFolder, hash),
	}
	//if thecontent doesn't exists, or its too old to be used,
	//we will request it to the main server
	if !ok ||
		time.Now().Sub(cacheTime).Nanoseconds() > s.Timeout.Nanoseconds() {
		s.l.Println("Requesting to main server..", s.RemoteAddress, r.RequestURI)
		client := http.Client{Timeout: s.Timeout, Transport: tr}
		uri := ""
		if strings.Contains(s.RemoteAddress, httpProto) || strings.Contains(s.RemoteAddress, httpsProto) {
			uri = s.RemoteAddress + r.RequestURI
		} else {
			uri = fmt.Sprintf("http://%s/%s", s.RemoteAddress, r.RemoteAddr)
		}
		var (
			resp *http.Response
			err  error
		)
		resp, err = s.request(uri, buf, contentSize, r, client)
		if err != nil {
			s.l.Println("Request to " + uri + " failed: " + err.Error())
			return
		}
		//write all request/response info in files to be reusable
		s.writeFiles(r, resp, buf, f)
		s.mapTimeouts[hash] = time.Now()
	}
	//writes the content information from the files to the actual response
	s.readHeadersFromFile(w, f.responseHeaders)
	err := s.serveFile(w, f.responseFile)
	if err != nil {
		s.l.Println("Error serving response file ", err)
	}
}

func (s *server) serveFile(w http.ResponseWriter, fn string) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func (s *server) writeFiles(r *http.Request, resp *http.Response, buf bytes.Buffer, f requestInfo) {
	err := writeHeadersToFile(r.Header, f.requestHeaders)
	s.l.Println("Writing request headers", err)
	err = writeToFile(resp.Body, f.responseFile)
	s.l.Println("Writing response", err)
	err = writeToFile(&buf, f.requestFile)
	s.l.Println("Writing request", err)
	err = writeResponseHeadersToFile(resp.Header, resp.StatusCode, f.responseHeaders)
	s.l.Println("Writing response headers", err)
}

func (s *server) copyHeaders(dst, src *http.Request) {
	for h, vs := range src.Header {
		for _, v := range vs {
			dst.Header.Add(h, v)
		}
	}
	s.l.Println("Final headers")
	s.l.Println(dst.Header)
}

func (s *server) readHeadersFromFile(w http.ResponseWriter, filepath string) error {
	var resHeader http.Header
	f, err := os.Open(filepath)
	if err != nil {
		s.l.Println("error reading response headers file: ", err)
		return err
	}
	err = json.NewDecoder(f).Decode(&resHeader)
	if err != nil {
		s.l.Println("error decoding response headers file: ", err)
		return err
	}
	for header, vs := range resHeader {
		if header == Status {
			continue
		}
		for _, v := range vs {
			w.Header().Add(header, v)
		}
	}
	st := resHeader.Get(Status)
	status, _ := strconv.Atoi(st)
	if status != 0 {
		w.WriteHeader(status)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	return nil
}

func writeResponseHeadersToFile(h http.Header, status int, filepath string) error {
	if h == nil {
		h = make(http.Header)
	}
	h["Status"] = []string{fmt.Sprint(status)}
	return writeHeadersToFile(h, filepath)
}

func writeHeadersToFile(h http.Header, filepath string) error {
	os.Remove(filepath)
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(h)
	if err != nil {
		return err
	}
	return f.Close()
}

func writeToFile(content io.Reader, filepath string) error {
	os.Remove(filepath)
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, content)
	if err != nil {
		return err
	}
	err = f.Close()
	return err
}
