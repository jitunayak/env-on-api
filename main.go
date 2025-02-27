package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"env-on-restapi/constants"
	"errors"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/atotto/clipboard"
	"github.com/fatih/color"
	"github.com/go-co-op/gocron"
)

var API_KEY string = ""

type AppConfigProperties map[string]string

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length+2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[2 : length+2]

}

func main() {
	shouldStartServer := flag.Bool("server", false, "starts the server")
	cron := flag.Bool("cron", false, "runs cron job")
	interval := flag.Int("interval", 10, "interval for cron job")
	command := flag.String("cmd", "echo no commands passed to run", "command to run periodically")
	portNumber := flag.String("port", "8088", "server port")
	shell := flag.String("shell", getCurrentShell(), "e.g. bash, powershell")
	flag.Parse()
	port := fmt.Sprintf(":%s", *portNumber)

	fmt.Println(len(os.Args), os.Args)

	defer color.Unset()

	if *shouldStartServer {
		bgYellow := color.New(color.FgWhite).Add(color.Bold).Add(color.BgYellow)

		API_KEY = string(randomString(40))
		clipboard.WriteAll(API_KEY)
		// bgYellow.Printf("\n 🦄 starting blazing fast web server on port %v \n\n", port)
		// color.Red("server started at port %v 🔥 \n\n", port)
		color.Yellow("GET - http://localhost%v/aws\n", port)
		bgYellow.Printf("API KEY: %v\n", API_KEY)
		color.White(constants.Title)
		color.Green(constants.Sample_code)
		startWebServer(port)

	} else {
		fmt.Println("You are on command line. Use eli --help to know all parameters")
		if *cron {
			s := gocron.NewScheduler(time.UTC)
			startCronJobInShell(s, *command, *interval, *shell)
			s.StartBlocking()
		}
	}
}

func startWebServer(port string) {

	config := AppConfigProperties{}

	http.HandleFunc("/aws", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		userKey := r.Header.Get("API-KEY")
		if userKey == "" {
			http.Error(w, "API-KEY missing from header", http.StatusBadRequest)
			return
		}
		if userKey != API_KEY {
			http.Error(w, "API-KEY incorrect from header", http.StatusBadRequest)
			return
		}
		shoudlReAuthenticate := r.URL.Query().Get("reAuthenticate")
		interval := r.URL.Query().Get("interval") //Interval in seconds
		command := r.URL.Query().Get("command")
		readType := r.URL.Query().Get("readType") //Optional Default value is file.All Possible Values 'file' | 'env'
		shell := r.URL.Query().Get("shell")       //Optional
		// catchTime := r.URL.Query().Get("catchTime")

		if shoudlReAuthenticate == "" {
			shoudlReAuthenticate = "false"
		}
		if interval == "" {
			interval = "3000" //Default Time Out : 50 minutes
		}
		if command == "" && shoudlReAuthenticate == "true" {
			http.Error(w, "command is missing to authenticate", http.StatusBadRequest)
		}
		if readType == "" {
			readType = "file"
		}
		if shell == "" {
			shell = getCurrentShell()
		}

		if shoudlReAuthenticate == "true" {
			intervalNumber, err := strconv.Atoi(interval)
			if err != nil {
				panic("interval time is invaliad")
			}
			s := gocron.NewScheduler(time.UTC)
			startCronJobInShell(s, command, intervalNumber, shell)
			s.StartAsync()
		}

		if shoudlReAuthenticate == "false" && command != "" {
			runOnShell(command, shell)
		}

		if readType == "file" {
			config := getAwsConfiguration(config)
			data := map[string]interface{}{
				"accessKeyId":  config["aws_access_key_id"],
				"secretKey":    config["aws_secret_access_key"],
				"sessionToken": config["aws_session_token"],
			}
			jsonData, err := json.Marshal(data)
			if err != nil {
				fmt.Printf("could not marshal json: %s\n", err)
				return
			}
			w.Write(jsonData)
		}
		if readType == "env" {
			data := map[string]interface{}{
				"accessKeyId":  os.Getenv("AWS_ACCESS_KEY_ID"),
				"secretKey":    os.Getenv("AWS_SECRET_ACCESS_KEY"),
				"sessionToken": os.Getenv("AWS_SESSION_TOKEN"),
			}

			jsonData, err := json.Marshal(data)
			if err != nil {
				fmt.Printf("could not marshal json: %s\n", err)
				return
			}
			w.Write(jsonData)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")

		var userRequest = make(map[string]string)
		var data = make(map[string]string)

		err := json.NewDecoder(r.Body).Decode(&userRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for i, request := range userRequest {
			env := os.Getenv(request)
			data[i] = env
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			fmt.Printf("could not marshal json: %s\n", err)
			return
		}
		w.Write(jsonData)
	})

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}

}

func getAwsCredentialFilePath() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	awsCredPath := filepath.Join(userHomeDir, ".aws", "credentials")
	return awsCredPath
}
func getAwsConfiguration(config AppConfigProperties) AppConfigProperties {
	awsCredPath := getAwsCredentialFilePath()
	file, err := os.Open(awsCredPath)

	if err != nil {
		log.Fatal(err)
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if equal := strings.Index(line, "="); equal >= 0 {
			if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
				value := ""
				if len(line) > equal {
					value = strings.TrimSpace(line[equal+1:])
				}
				config[key] = value
			}
		}
	}
	return config
}

func getCurrentShell() string {
	switch runtime.GOOS {
	case "windows":
		return "powershell"
	case "darwin":
		return "zsh"
	case "linux":
		return "bash"
	default:
		log.Fatal("no shell found to execute command")
		return ""
	}
}

func runOnShell(command string, shell string) {
	log.Printf("running : %s", command)
	cmd := exec.Command(shell, "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func startCronJobInShell(s *gocron.Scheduler, command string, interval int, shell string) {
	color.Green("interval: ", interval, "seconds")
	color.Green("command: ", command)
	if s.IsRunning() {
		s.Stop()
	}
	fmt.Println("\nstarted corn job")

	s.Every(interval).Seconds().Do(func() {
		runOnShell(command, shell)

	})
}

// ! TODO
func getEliConfigurationPath() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join("", userHomeDir, ".eli", "configuration")
}

// ! TODO
func readConfiguration() {

	if _, err := os.Stat(getEliConfigurationPath()); err == nil {
		os.ReadFile(getEliConfigurationPath())
	}
}

// ! TODO
func updateConfiguration(config string) {

	if _, err := os.Stat(getEliConfigurationPath()); err == nil {

	} else if errors.Is(err, os.ErrNotExist) {
		// os.Mkdir(filepath.Join(userHomeDir, ".eli"), os.ModePerm)
		f, err := os.Create(getAwsCredentialFilePath())
		if err != nil {
			log.Fatal(err)
		}
		f.WriteString(config)
		defer f.Close()
	}
}
