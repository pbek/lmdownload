package main

import (
	"github.com/henrylee2cn/surfer"
	"log"
	"io/ioutil"
	"regexp"
	"os"
	"github.com/go-ini/ini"
	"bufio"
	"flag"
	"fmt"
	"strings"
	"golang.org/x/crypto/ssh/terminal"
	"syscall"
	"path/filepath"
	"os/user"
)

const pageUrl  = "https://www.linux-magazine.com"

const iniFileName = "lmdownload.ini"
// will be overwritten by $SNAP_USER_COMMON
const relativeIniDirectoryPath = ".local/share/lmdownload"
const settingsKey = ""
const usernameSettingsKey = "Username"
const passwordSettingsKey = "Password"
const concurrentDownloads = 3

// this variable will be overwritten in the build process
var version = "current"

var username string
var password string
var cfg *ini.File
var err error
var reader *bufio.Reader
var req *surfer.Request
var forceLogin bool
var iniPath string
var latestOnly bool

func main() {
	flag.StringVar(&username,"username", "", "Username to Linux Magazine")
	flag.StringVar(&password,"password", "", "Password to Linux Magazine")
	flag.BoolVar(&forceLogin,"login", false, "Force to enter login data again")
	flag.BoolVar(&latestOnly,"latest-only", false, "Download only the latest PDF")
	showVersion := flag.Bool( "v", false, "Show version number")
	flag.Parse()

	if *showVersion {
		fmt.Println("Linux Magazine Downloader version: ", version)
		os.Exit(0)
	}

	reader = bufio.NewReader(os.Stdin)

	if !createIniFileIfNotExists() {
		os.Exit(1)
	}

	cfg, err = ini.Load(iniPath)
	if err != nil {
		log.Fatal("Failed to read ini file: ", err)
		os.Exit(1)
	}

	readUsername()
	readPassword()
	downloadPDFs()
}

func downloadPDFs() {
	values := map[string][]string{
		"Login":       {username},
		"Password":    {password},
		"LoginButton": {"Login"},
		"RedirectURI": {"lnmshop/account"},
	}

	var form = surfer.Form{Values: values}
	log.Println("Doing page login and fetching available PDFs...")

	req = &surfer.Request{
		Url:          pageUrl + "/user/login",
		Method:       "POST",
		Body:         form,
		EnableCookie: true,
	}

	body, err := req.ReadBody()
	resp, err := surfer.Download(req)

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	body, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	// check if login failed
	loginFailedRegExp := regexp.MustCompile(`A valid username and password is required to login.`)
	if loginFailedRegExp.Match(body) {
		log.Fatal("Login failed! Please check your username/password!")
		os.Exit(2)
	}

	// find PDFs to download
	pdfRegExp := regexp.MustCompile(`<a href="(.+\.pdf)">PDF file</a>`)
	matches := pdfRegExp.FindAllStringSubmatch(string(body[:]), -1)
	pdfFileNameRegExp := regexp.MustCompile(`[^/]+\.pdf$`)
	jobCount := 0
	jobs := make(chan [2]string, len(matches))
	results := make(chan bool, len(matches))

	// start the download workers
	for w := 1; w <= concurrentDownloads; w++ {
		go downloadWorker(w, jobs, results)
	}

	// add the download jobs
	for matchKey, match := range matches {
		// break after the first download if we only need the first PDF
		if latestOnly && matchKey > 0 {
			break
		}

		pdfUrl := pageUrl + match[1];
		pdfFileName := pdfFileNameRegExp.FindString(pdfUrl)

		if pdfFileName == "" {
			log.Fatalf("No filename was found in url <%s>", pdfUrl)
			continue
		}

		// check if file already exists
		if _, err := os.Stat(pdfFileName); err == nil {
			log.Println("File already found:", pdfFileName)
			continue
		}

		jobs <- [2]string{pdfUrl, pdfFileName}
		jobCount++
	}

	close(jobs)

	// wait for the results
	for c := 1; c <= jobCount; c++ {
		<-results
	}

	log.Println("Done")
}

/**
 * The download worker downloads PDFs from the job queue
 */
func downloadWorker(id int, jobs <-chan [2]string, results chan<- bool) {
	for j := range jobs {
		pdfUrl := j[0]
		pdfFileName := j[1]
		log.Println("worker", id, "started to download", pdfUrl)

		// download pdf file
		req := &surfer.Request{
			Url:          pdfUrl,
			EnableCookie: true,
		}

		body, err := req.ReadBody()
		resp, err := surfer.Download(req)

		if err != nil {
			log.Fatal(err)
			results <- false
			continue
		}

		body, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Fatal(err)
			results <- false
			continue
		}

		err = ioutil.WriteFile(pdfFileName, body, 0644)

		if err != nil {
			log.Fatal(err)
			results <- false
			continue
		}

		file, _ := filepath.Abs(pdfFileName)
		log.Println("worker", id, "finished job and stored pdf", file)

		results <- true
	}
}

/**
 * Reads and stores the username
 */
func readUsername() {
	storeSettings := true

	if username == "" {
		username = cfg.Section(settingsKey).Key(usernameSettingsKey).String()
	}

	if username == "" || forceLogin {
		fmt.Print("Enter Username: ")
		username, _ = reader.ReadString('\n')
		username = strings.TrimSpace(username)
	} else {
		storeSettings = false
	}

	if username == "" {
		log.Fatalf("Please provide a username!")
		os.Exit(1)
	} else if storeSettings {
		cfg.Section(settingsKey).Key(usernameSettingsKey).SetValue(username)
		cfg.SaveTo(iniPath)
	}
}

/**
 * Reads and stores the password
 */
func readPassword() {
	storeSettings := true

	if password == "" {
		password = cfg.Section(settingsKey).Key(passwordSettingsKey).String()
	}

	if password == "" || forceLogin {
		fmt.Print("Enter Password: ")
		bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
		password = strings.TrimSpace(string(bytePassword))
		fmt.Println()
	} else {
		storeSettings = false
	}

	if password == "" {
		log.Fatalf("Please provide a password!")
		os.Exit(1)
	} else if storeSettings {
		cfg.Section(settingsKey).Key(passwordSettingsKey).SetValue(password)
		cfg.SaveTo(iniPath)
	}
}

/**
 * Creates the ini file if it doesn't exist
 */
func createIniFileIfNotExists() bool {
	var usr *user.User

	usr, err = user.Current()
	if err != nil {
		log.Fatal( err )
		return false
	}

	// try to use the snap common path for the settings
	iniDirectoryPath := os.Getenv("SNAP_USER_COMMON")

	if iniDirectoryPath == "" {
		// set the ini directory path
		iniDirectoryPath = usr.HomeDir + string(os.PathSeparator) + relativeIniDirectoryPath
	}

	err = os.MkdirAll(iniDirectoryPath, 0700)

	if err != nil {
		log.Fatal(err)
		return false
	}

	// set the ini path
	iniPath = iniDirectoryPath + string(os.PathSeparator) + iniFileName

	// detect if file exists
	var _, err = os.Stat(iniPath)

	// create file if not exists
	if os.IsNotExist(err) {
		var file, err = os.OpenFile(iniPath, os.O_RDWR|os.O_CREATE, 0600)

		if err != nil {
			log.Fatal(err)
			return false
		}

		log.Println("ini file ", iniPath, " was created")
		defer file.Close()
	}

	return true
}