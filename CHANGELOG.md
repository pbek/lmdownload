# Linux Magazine Downloader Changelog

## 20.12.1
- added the parameter `--smtp-host`

## 20.12.0
- migration to go modules

## 18.04.4
- the password is now stored encrypted in the ini-file
    - keep in mind that you now will be asked for your password again once if you have already stored it

## 18.04.3
- added the parameter `--notification-email=your@email.com` to send a notifcation email via `localhost`

## 18.04.2
- the PDFs are now downloaded with three download workers that will automatically
  pick up a new download if they finished downloading a file
- added the parameter `--latest-only` to only download the latest PDF

## 18.04.1
- settings will now be stored to `~/.local/share/lmdownload/lmdownload.ini`
    - if you are using the *snap* settings will be stored to `~/snap/lmdownload/common/lmdownload.ini`

## 18.04.0
- PDFs will be downloaded to the current directory
    - if you are installing the *snap* be sure to be inside your home directory
- use the `--login` parameter if you want to change your username / password
