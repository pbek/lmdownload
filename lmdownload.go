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
)

const pageUrl  = "https://www.linux-magazine.com"

const iniPath = "lmdownload.ini"
//const settingsKey = "LinuxMagazine"
const settingsKey = ""
const usernameSettingsKey = "Username"
const passwordSettingsKey = "Password"

var username string
var password string
var cfg *ini.File
var err error
var reader *bufio.Reader

func main() {
	reader = bufio.NewReader(os.Stdin)

	if !createIniFileIfNotExists() {
		os.Exit(1)
	}

	cfg, err = ini.Load(iniPath)
	if err != nil {
		log.Fatal("Failed to read ini file: ", err)
		os.Exit(1)
	}

	flag.StringVar( &username,"username", "", "Username to Linux Magazine.")
	flag.StringVar( &password,"password", "", "Password to Linux Magazine.")
	flag.Parse()

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
	req := &surfer.Request{
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
	for _, value := range matches {
		pdfUrl := pageUrl + value[1];
		pdfFileName := pdfFileNameRegExp.FindString(pdfUrl)

		if pdfFileName == "" {
			log.Fatal("No filename was found in url <%s>", pdfUrl)
			continue
		}

		log.Printf("Downloading <%s> as '%s'...", pdfUrl, pdfFileName)

		// download pdf file
		req = &surfer.Request{
			Url:          pdfUrl,
			EnableCookie: true,
		}

		body, err = req.ReadBody()
		resp, err = surfer.Download(req)

		if err != nil {
			log.Fatal(err)
			continue
		}

		body, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Fatal(err)
			continue
		}

		err = ioutil.WriteFile(pdfFileName, body, 0644)

		if err != nil {
			log.Fatal(err)
			continue
		}

		// Todo: remove
		break
	}

	log.Println("Done")
}

/**
 * Reads and stores the username
 */
func readUsername() {
	storeSettings := true

	if username == "" {
		username = cfg.Section(settingsKey).Key(usernameSettingsKey).String()

		if username != "" {
			storeSettings = false
		}
	}

	if username == "" {
		fmt.Print("Enter Username: ")
		username, _ = reader.ReadString('\n')
		username = strings.TrimSpace(username)
	}

	if username == "" {
		log.Fatalf("Please provide a username in ini file '%v' with settings key '%v'", iniPath, usernameSettingsKey)
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

		if password != "" {
			storeSettings = false
		}
	}

	if password == "" {
		fmt.Print("Enter Password: ")
		bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
		password = strings.TrimSpace(string(bytePassword))
	}

	if password == "" {
		log.Fatalf("Please provide a password in ini file '%v' with settings key '%v'", iniPath, passwordSettingsKey)
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