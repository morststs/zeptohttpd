package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
)

type Config struct {
	ContentTypes map[string]string
}

func getConfig() (config Config, err error) {
	jsonFile, err := os.Open("config.json")
	if err != nil {
		fmt.Println("config.json open err : ", err)
		return Config{}, err
	}
	defer jsonFile.Close()
	jsonData, err := io.ReadAll(jsonFile)
	if err != nil {
		fmt.Println("config.json read err : ", err)
		return Config{}, err
	}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		fmt.Println("config.json parce : ", err)
		return Config{}, err
	}
	return config, nil
}

func startServer(config *Config) error {
	fileServer := http.FileServer(http.Dir("public"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for path, contentType := range config.ContentTypes {
			if strings.HasPrefix(r.URL.Path, path) {
				w.Header().Set("Content-Type", contentType)
			}
		}
		fmt.Printf("URL.Path : %s\n", html.EscapeString(r.URL.Path))
		fileServer.ServeHTTP(w, r)
	})
	return http.ListenAndServe(":http", &handler)
}

func main() {
	config, err := getConfig()
	if err != nil {
		return
	}
	if err := startServer(&config); err != nil {
		fmt.Print(err)
	}
}
