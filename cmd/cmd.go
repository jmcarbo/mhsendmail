package cmd

import (
  "bytes"
  "fmt"
  "io/ioutil"
  "log"
  "net/mail"
  "github.com/emersion/go-smtp"
  "os"
  "os/user"
  "strings"
  "github.com/emersion/go-message"
  "github.com/emersion/go-sasl"
  _ "github.com/emersion/go-message/charset"
  "regexp"
  "github.com/spf13/viper"
  flag "github.com/spf13/pflag"
)

// Go runs the MailHog sendmail replacement.
func Go() {
  host, err := os.Hostname()
  if err != nil {
    host = "localhost"
  }
  viper.SetDefault("host", host)

  username := "nobody"
  user, err := user.Current()
  if err == nil && user != nil && len(user.Username) > 0 {
    username = user.Username
  }

  fromAddr := username + "@" + host
  viper.SetDefault("from-addr", fromAddr)

  smtpAddr := "localhost:1025"
  viper.SetDefault("smtp-addr", smtpAddr)

  viper.SetConfigName("config") // name of config file (without extension)
  viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name
  viper.AddConfigPath("/etc/mhsendmail/")   // path to look for the config file in
  viper.AddConfigPath("$HOME/.mhsendmail")  // call multiple times to add many search paths
  viper.AddConfigPath(".")               // optionally look for config in the working directory
  err = viper.ReadInConfig() // Find and read the config file
  if err != nil { // Handle errors reading the config file
    panic(fmt.Errorf("Fatal error config file: %s \n", err))
  }

  var recip []string

  // defaults from envars if provided
  if len(os.Getenv("MH_SENDMAIL_SMTP_ADDR")) > 0 {
    smtpAddr = os.Getenv("MH_SENDMAIL_SMTP_ADDR")
  }
  if len(os.Getenv("MH_SENDMAIL_FROM")) > 0 {
    fromAddr = os.Getenv("MH_SENDMAIL_FROM")
  }

  var verbose bool
  verbose = viper.GetBool("verbose")
  if verbose {
    fmt.Fprintf(os.Stderr, "Smtp: [%s] From: [%s] To: [%v]\n", smtpAddr, fromAddr, recip)
  }

  // override defaults from cli flags
  flag.StringP("smtp-addr", "s", "", "SMTP server address")
  flag.StringP("from-addr", "f", "", "SMTP sender")
  flag.BoolP("long-i", "i", true, "Ignored. This flag exists for sendmail compatibility.")
  flag.BoolP("long-o", "o", true, "Ignored. This flag exists for sendmail compatibility.")
  flag.BoolP("long-t", "t", true, "Ignored. This flag exists for sendmail compatibility.")
  flag.BoolP("long-bs", "b", true, "Ignored. This flag exists for sendmail compatibility.")
  flag.BoolP("verbose", "v", false, "Verbose mode (sends debug output to stderr)")

  flag.Parse()
  viper.BindPFlags(flag.CommandLine)

  // allow recipient to be passed as an argument
  recip = flag.Args()

  smtpAddr = viper.GetString("smtp-addr")
  fromAddr = viper.GetString("from-addr")
  username = viper.GetString("username")
  password := viper.GetString("password")
  if verbose {
    fmt.Fprintf(os.Stderr, "Smtp: [%s] From: [%s] To: [%v]\n", smtpAddr, fromAddr, recip)
  }

  body, err := ioutil.ReadAll(os.Stdin)
  if err != nil {
    fmt.Fprintln(os.Stderr, "error reading stdin")
    os.Exit(11)
  }

  //msg, err := mail.ReadMessage(bytes.NewReader(body))
  msg, err := message.Read(bytes.NewReader(body))
  if err != nil {
    fmt.Fprintln(os.Stderr, "error parsing message body")
    os.Exit(11)
  }

  if len(recip) == 0 {
    re := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

    // We only need to parse the message to get a recipient if none where
    // provided on the command line.
    // REFACTORING: needs refactoring
    recipientss, err := msg.Header.Text("To")
    if err != nil {
      fmt.Fprintln(os.Stderr, "error parsing email recipient header"+ err.Error())
      os.Exit(11)
    }
    recipients := strings.Split(recipientss, ",")
    fmt.Printf("%+v\n", recipients)
    for i := range recipients {
      recipient := strings.TrimSpace(recipients[i])
      if len(recipient) == 0 {
        continue
      }
      parsed, err := mail.ParseAddress(recipient)
      if err != nil {
        if re.MatchString(recipient) {
          recip = append(recip, recipient)
        } else {
          fmt.Fprintf(os.Stderr, "[%s]\n", recipient)
          fmt.Fprintln(os.Stderr, "error parsing email recipient To "+ err.Error())
          os.Exit(11)
        }
      } else {
        recip = append(recip, parsed.Address)
      }
    }

    recipientss, err = msg.Header.Text("Cc")
    if err != nil {
      fmt.Fprintln(os.Stderr, "error parsing email recipient "+ err.Error())
      os.Exit(11)
    }
    recipients = strings.Split(recipientss, ",")
    for i := range recipients {
      recipient := strings.TrimSpace(recipients[i])
      if len(recipient) == 0 {
        continue
      }
      parsed, err := mail.ParseAddress(recipient)
      if err != nil {
        fmt.Fprintln(os.Stderr, recipient)
        fmt.Fprintln(os.Stderr, "error parsing email recipient Cc "+ err.Error())
        os.Exit(11)
      }
      recip = append(recip, parsed.Address)
    }

    recipientss, err = msg.Header.Text("Bcc")
    if err != nil {
      fmt.Fprintln(os.Stderr, "error parsing email recipient Bcc"+ err.Error())
      os.Exit(11)
    }
    recipients = strings.Split(recipientss, ",")
    for i := range recipients {
      recipient := strings.TrimSpace(recipients[i])
      if len(recipient) == 0 {
        continue
      }
      parsed, err := mail.ParseAddress(recipient)
      if err != nil {
        fmt.Fprintln(os.Stderr, recipient)
        fmt.Fprintln(os.Stderr, "error parsing email recipient" + err.Error())
        os.Exit(11)
      }
      recip = append(recip, parsed.Address)
    }
  }

  if verbose {
    fmt.Fprintf(os.Stderr, "From: [%s] To: [%v]\n", fromAddr, recip)
  }
  if username != "" {
    // Set up authentication information.
    auth := sasl.NewPlainClient("", username, password)
    err = smtp.SendMail(smtpAddr, auth, fromAddr, recip, bytes.NewReader(body))
  } else {
    err = smtp.SendMail(smtpAddr, nil, fromAddr, recip, bytes.NewReader(body))
  }
  if err != nil {
    fmt.Fprintln(os.Stderr, "error sending mail")
    log.Fatal(err)
  }

}
