package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"search/prototype/client"
	"search/prototype/logger"
	"search/prototype/server"
	"strconv"
	"strings"
	"time"
)

// Sets up the server-side flags
var numClients = flag.Int("num_clients", 5, "the number of clients for the server")
var lenMS = flag.Int("len_ms", 8, "the length of the master secret")
var lenSalt = flag.Int("len_salt", 8, "the length of the salts used to generate the PRFs")
var fpRate = flag.Float64("fp_rate", 0.000001, "the desired false positive rate for searchable encryption")
var numUniqWords = flag.Uint64("num_words", uint64(10000), "the expected number of unique words in all the documents")
var serverDirectory = flag.String("server_mp", ".server_fs", "the directory for the server where all the server side data is stored")

// Sets up the client-side flags
var defaultClientNum = flag.Int("default_client_num", 0, "the dafault running client (set to -1 to initialize without a client)")
var clientDirectory = flag.String("client_mp", ".client_fs", "the directory for the client where the client stores all the data")

// Sets up the logger
var enableLogger = flag.Bool("enable_logger", true, "whether time logging should be enabled")
var latency = flag.Int64("latency", 100, "the latency between the server and the client (in ms)")
var bandwidth = flag.Int("bandwidth", 1024*1024, "the bandwidth between the server and the client (in bps)")

// startServer initializes the server for the program.  It either creates a new
// one or loads from the server metadata at the directory.
func startServer() (*server.Server, error) {
	if _, err := os.Stat(path.Join(*serverDirectory, "serverMD")); err == nil {
		fmt.Println("Server metadata found, loading server from directory", *serverDirectory)
		return server.LoadServer(*serverDirectory), nil
	}
	if _, err := os.Stat(*serverDirectory); os.IsNotExist(err) {
		if os.Mkdir(*serverDirectory, 0777) != nil {
			fmt.Println("Failed to create the server directory", *serverDirectory)
		}
	}
	fmt.Println("No previous server metadata found, starting new server at directory", *serverDirectory)
	return server.CreateServerWithNetConfig(*numClients, *lenMS, *lenSalt, *serverDirectory, *fpRate, *numUniqWords, time.Millisecond*time.Duration(*latency), *bandwidth)
}

// startClient initializes a client with `clientNum` connected to `server`.
func startClient(server *server.Server, clientNum int) *client.Client {
	if clientNum == -1 {
		fmt.Println("No client running")
		return nil
	}
	fmt.Println("Now running client", clientNum)
	return client.CreateClient(server, clientNum, path.Join(*clientDirectory, "client"+strconv.Itoa(clientNum)))
}

// addFile adds `file` to `client` if `file` exists and has not already been
// added.
func addFile(client *client.Client, file string) {
	_, filename := path.Split(file)
	err := client.AddFile(file)
	if err == nil {
		fmt.Printf("File %s successfully added\n", filename)
	} else {
		fmt.Printf("Cannot add file %s: %s\n", filename, err)
	}
}

func addDirectory(client *client.Client) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			addFile(client, path)
		} else if info.Name()[0] == '.' && info.Name() != "." {
			return filepath.SkipDir
		}
		return nil
	}
}

// A list of commands:
//	-client/c X
//			Starts running client with client number X
//	-ls/l
//			Lists all the files on the server
//	-search/s w1 w2 w3 ...
//			Searches the words in the server
//	-searchn/sn w1 w2 w3 ...
//			Searches the words in the server (naive version)
//	-add/a f1 f2 d1 d2 ...
//			Adds the files and directories (recursive) to the system
//	-info/i
//			Prints the server information
//	-exit/q
//			Exits the program
func main() {
	flag.Parse()

	// Initialize the server
	server, serverErr := startServer()
	if serverErr != nil {
		fmt.Println("Cannot start the server")
		return
	}
	fmt.Printf("\nServer Started\n--------------\n")
	server.PrintServerInfo()
	fmt.Println()

	// Initialize the client
	if _, err := os.Stat(*clientDirectory); os.IsNotExist(err) {
		if os.Mkdir(*clientDirectory, 0777) != nil {
			fmt.Println("Failed to create the client directory", *clientDirectory)
		}
	}
	client := startClient(server, *defaultClientNum)
	fmt.Println()

	// Initializes the logger
	if *enableLogger {
		logger.Enable()
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		cmd, _ := reader.ReadString('\n')
		cmd = cmd[:len(cmd)-1]
		tokens := strings.Split(cmd, " ")
		logger.Start(tokens[0])
		switch tokens[0] {
		case "client", "c":
			if len(tokens) < 2 {
				fmt.Printf("%s: client number missing\n", tokens[0])
				break
			}
			clientNum, err := strconv.Atoi(tokens[1])
			if err != nil || clientNum < 0 || clientNum >= server.GetNumClients() {
				fmt.Printf("%s: invalid client number \"%s\"\n", tokens[0], tokens[1])
				break
			}
			client = startClient(server, clientNum)
		case "ls", "l":
			if client == nil {
				fmt.Printf("%s: client not running\n", tokens[0])
				break
			}
			for _, filename := range client.GetFilenames() {
				fmt.Println(filename)
			}
		case "search", "s":
			if client == nil {
				fmt.Printf("%s: client not running\n", tokens[0])
				break
			}
			if len(tokens) < 2 {
				fmt.Printf("%s: search keyword missing\n", tokens[0])
				break
			}
			for i := 1; i < len(tokens); i++ {
				fmt.Printf("Search result for %s:\n", tokens[i])
				filenames, fpRate, err := client.SearchWord(tokens[i])

				if err != nil {
					fmt.Printf("\tError when searching word: %s\n", err)
				}
				if len(filenames) == 0 {
					fmt.Printf("\tNo file contains the word \"%s\"\n", tokens[i])
				}
				for _, filename := range filenames {
					fmt.Printf("\t%s\n", filename)
				}
				fmt.Printf("False Positive Rate: %f%%\n", fpRate*100)
			}
		case "searchn", "sn":
			if client == nil {
				fmt.Printf("%s: client not running\n", tokens[0])
				break
			}
			if len(tokens) < 2 {
				fmt.Printf("%s: search keyword missing\n", tokens[0])
				break
			}
			for i := 1; i < len(tokens); i++ {
				fmt.Printf("Search result for %s:\n", tokens[i])
				filenames, _, err := client.SearchWordNaive(tokens[i])

				if err != nil {
					fmt.Printf("\tError when searching word: %s\n", err)
				}
				if len(filenames) == 0 {
					fmt.Printf("\tNo file contains the word \"%s\"\n", tokens[i])
				}
				for _, filename := range filenames {
					fmt.Printf("\t%s\n", filename)
				}
			}
		case "add", "a":
			if client == nil {
				fmt.Printf("%s: client not running\n", tokens[0])
				break
			}
			if len(tokens) < 2 {
				fmt.Printf("%s: file name missing\n", tokens[0])
				break
			}
			for i := 1; i < len(tokens); i++ {
				info, err := os.Stat(tokens[i])
				if os.IsNotExist(err) {
					fmt.Println("Invalid path", tokens[i])
					continue
				}
				if info.IsDir() {
					filepath.Walk(tokens[i], addDirectory(client))
				} else {
					addFile(client, tokens[i])
				}
			}

		case "info", "i":
			server.PrintServerInfo()
		case "exit", "q":
			fmt.Println("Program exited.")
			return
		case "":
		default:
			fmt.Printf("%s: command not found\n", tokens[0])
		}
		logger.Log(tokens[0])
	}
}
