package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
)

type serverInfo struct {
	RemoteAddress string `json:"remote"`
	LocalPort     string `json:"local"`
	Timeout       int    `json:"timeout_minutes"`
	CachePath     string `json:"cache_path"`
	Name          string `json:"name"`
}

var (
	configFile = flag.String("config", "", ".json file that contains the server definitions")
)

func main() {
	flag.Parse()
	if *configFile == "" {
		flag.Usage()
		log.Fatalln("Config file is required")
	}
	jsonContent, err := ioutil.ReadFile(*configFile)
	panicWith(err, "Cannot read file content")

	var servers []serverInfo
	err = json.
		NewDecoder(bytes.NewBuffer(jsonContent)).
		Decode(&servers)
	panicWith(err, "Couldn't parse json file. It must be a server array")

	wait := make(chan struct{})
	initServers(servers)
	log.Println("Servers ready!")
	<-wait
}

func initServers(servers []serverInfo) {
	for _, config := range servers {
		serve(config)
	}
}

func panicWith(err error, message string) {
	if err != nil {
		log.Fatalln(message, err)
	}
}
