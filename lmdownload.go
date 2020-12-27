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
	"gopkg.in/gomail.v2"
	"crypto/tls"
	"io"
	"crypto/rand"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"encoding/base64"
)

const pageUrl  = "https://www.linux-magazine.com"

const iniFileName = "lmdownload.ini"
// will be overwritten by $SNAP_USER_COMMON
const relativeIniDirectoryPath = ".local/share/lmdownload"
const settingsKey = ""
const usernameSettingsKey = "Username"
const passwordSettingsKey = "Password"
const encryptionKeySettingsKey = "Key"
const concurrentDownloads = 3

// this variable will be overwritten in the build process
var version = "current"

var username string
var password string
var encryptionKey = &[32]byte{}
var cfg *ini.File
var err error
var reader *bufio.Reader
var req *surfer.Request
var forceLogin bool
var iniPath string
var latestOnly bool
var notificationEmail string
var smtpHost string

func main() {
	flag.StringVar(&username,"username", "", "Username to Linux Magazine")
	flag.StringVar(&password,"password", "", "Password to Linux Magazine")
	flag.BoolVar(&forceLogin,"login", false, "Force to enter login data again")
	flag.BoolVar(&latestOnly,"latest-only", false, "Download only the latest PDF")
	flag.StringVar(&notificationEmail,"notification-email", "", "Email address to send notification to")
	flag.StringVar(&smtpHost,"smtp-host", "localhost", "SMTP server to send emails")
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

	readEncryptionKey()
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
	results := make(chan string, len(matches))

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
	var fileList []string;

	// wait for the results
	for c := 1; c <= jobCount; c++ {
		fileList = append(fileList, <-results)
	}

	if notificationEmail != "" && len(fileList) > 0 {
		sendNotification(fileList)
	}

	log.Println("Done")
}

/**
 * The download worker downloads PDFs from the job queue
 */
func downloadWorker(id int, jobs <-chan [2]string, results chan<- string) {
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
			results <- ""
			continue
		}

		body, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Fatal(err)
			results <- ""
			continue
		}

		err = ioutil.WriteFile(pdfFileName, body, 0644)

		if err != nil {
			log.Fatal(err)
			results <- ""
			continue
		}

		file, _ := filepath.Abs(pdfFileName)
		log.Println("worker", id, "finished job and stored pdf", file)

		results <- file
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
		password = decryptText(cfg.Section(settingsKey).Key(passwordSettingsKey).String())
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
		cfg.Section(settingsKey).Key(passwordSettingsKey).SetValue(encryptText(password))
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

/**
 * Sends a notification email
 */
func sendNotification(fileList []string) {
	body := fmt.Sprintf("%v files were downloaded<ul>", len(fileList))

	for _, file := range fileList {
		body += "<li>" + file + "</li>"
	}

	body += "</ul>Your <a href=\"https://github.com/pbek/lmdownload\">Linux Magazine Downloader</a>"

	m := gomail.NewMessage()
	m.SetHeader("From", notificationEmail)
	m.SetHeader("To", notificationEmail)
	m.SetHeader("Subject", "Linux Magazine Downloader")
	m.SetBody("text/html", body)

	d := gomail.Dialer{Host: smtpHost, Port: 25}
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	log.Println("Sending notification email to", notificationEmail, "(check your spam folder)...")

	if err := d.DialAndSend(m); err != nil {
		log.Fatal(err)
	}
}

/**
 * Loads or generates an stores the encryption key
 */
func readEncryptionKey() {
	keyText := cfg.Section(settingsKey).Key(encryptionKeySettingsKey).String()

	if keyText != "" {
		key, err := base64.StdEncoding.DecodeString(keyText)

		if err != nil {
			log.Println("encryption code decode error:", err)
		} else {
			copy(encryptionKey[:], key[0:32])
			return
		}
	}

	_, err := io.ReadFull(rand.Reader, encryptionKey[:])

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	keyText = base64.StdEncoding.EncodeToString(encryptionKey[:])
	cfg.Section(settingsKey).Key(encryptionKeySettingsKey).SetValue(keyText)
	cfg.SaveTo(iniPath)
}

func encryptText(text string) string {
	ciphertext, err := Encrypt([]byte(text), encryptionKey)
	if err != nil {
		log.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(ciphertext)
}

func decryptText(cipherText string) string {
	data, err := base64.StdEncoding.DecodeString(cipherText)

	if err != nil {
		log.Fatal("base64 decode error:", err)
		return ""
	}

	text, err := Decrypt(data, encryptionKey)
	if err != nil {
		return ""
	}

	return string(text)
}

// Encrypt encrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Output takes the
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Encrypt(plaintext []byte, key *[32]byte) (ciphertext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Expects input
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Decrypt(ciphertext []byte, key *[32]byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}

	return gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
}
