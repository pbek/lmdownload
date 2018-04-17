# Linux Magazine Downloader Changelog

## 18.04.2
- the PDFs are now downloaded with three download workers that will automatically
  pick up a new download if they finished downloading a file

## 18.04.1
- settings will now be stored to `~/.local/share/lmdownload/lmdownload.ini`
    - if you are using the *snap* settings will be stored to `~/snap/lmdownload/common/lmdownload.ini`

## 18.04.0
- PDFs will be downloaded to the current directory
    - if you are installing the *snap* be sure to be inside your home directory
- use the `--login` parameter if you want to change your username / password
