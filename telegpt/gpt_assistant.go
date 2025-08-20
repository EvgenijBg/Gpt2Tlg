package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sync"

	// aitools "telegpt/OpenAITools/OpenAI"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	assistantID        string
	threadsFileName    string
	activeBots         []string
	userAddBotRequests map[string]bool // Map to track user requests for adding bots
)

type BotThreads struct {
	sync.Mutex
	BotName string
	Threads map[string]string
}
type ThreadStore struct {
	sync.Mutex
	Threads []BotThreads
}

type UserRequest struct {
	UserMessage string `json:"message"`
	UserID      string `json:"uuid"`
}

type WorkerTask struct {
	UserMessage string
	ThreadID    string
}

type BotToken struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

type Config struct {
	ApiKey          string     `json:"apiKey"`
	AssistantID     string     `json:"assistantID"`
	ManagerBotToken string     `json:"managerBotToken"`
	BotTokens       []BotToken `json:"botTokens"`
	ThreadsFileName string     `json:"threadsFileName"`
}

func NewThreadStore() *ThreadStore {
	return &ThreadStore{
		Threads: make([]BotThreads, 0),
	}
}

func (s *ThreadStore) SaveThread(botName string, userID string, threadID string) {
	s.Lock()
	defer s.Unlock()
	s.Threads = append(s.Threads, BotThreads{
		BotName: botName,
		Threads: map[string]string{
			userID: threadID,
		},
	})
}

func (s *ThreadStore) GetThread(botName string, userID string) (string, bool) {
	s.Lock()
	defer s.Unlock()
	for i := range s.Threads {
		thread := &s.Threads[i]
		if thread.BotName == botName {
			if threadID, exists := thread.Threads[userID]; exists {
				return threadID, true
			}
		}
	}
	return "", false
}

func (s *ThreadStore) DeleteThread(botName string, userID string) {
	s.Lock()
	defer s.Unlock()
	for i := range s.Threads {
		thread := &s.Threads[i]
		if thread.BotName == botName {
			delete(thread.Threads, userID)
			if len(thread.Threads) == 0 {
				s.Threads = append(s.Threads[:i], s.Threads[i+1:]...)
			}
			break
		}
	}
}

var threadStore = NewThreadStore()

func main() {
	config := readConfig()
	apiKey = config.ApiKey // Set apiKey for OpenAI.go
	assistantID = config.AssistantID
	managerBotToken := config.ManagerBotToken
	botTokens := config.BotTokens
	threadsFileName = config.ThreadsFileName
	fmt.Printf("APIKEY: ~%s~\n", apiKey)
	fmt.Printf("ASSID: ~%s~\n", assistantID)
	ImportThreads(threadsFileName)
	logstream := make(chan string)
	defer close(logstream)
	go fileWriter(logstream, "./log.txt")
	go startManagerBot(logstream, managerBotToken)
	for _, bot := range botTokens {
		fmt.Printf("Starting bot %s with token: %s\n", bot.Name, bot.Token)
		activeBots = append(activeBots, bot.Name)
		go startClientBot(logstream, bot.Token)
	}
	select {}
}

func startManagerBot(logstream chan string, botToken string) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)
	config := readConfig()
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	for {
		updates := bot.GetUpdatesChan(u)

		for update := range updates {
			if update.Message != nil {
				if update.Message.IsCommand() {
					switch update.Message.Command() {
					case "reload":
						// Reload configuration
						config = readConfig()
						botTokens := config.BotTokens
						threadsFileName = config.ThreadsFileName

						// Restart all bots
						for _, bot := range botTokens {
							if !slices.Contains(activeBots, bot.Name) {
								activeBots = append(activeBots, bot.Name)
								go startClientBot(logstream, bot.Token)
							} else {
								log.Printf("Bot %s is already active.", bot.Name)
							}
						}

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Configuration reloaded and bots restarted.")
						if _, err := bot.Send(msg); err != nil {
							log.Printf("Error sending reload confirmation: %v", err)
						}
					case "list":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Active bots:")
						for _, bot := range activeBots {
							msg.Text += fmt.Sprintf("\n- @%s", bot)
						}
						if _, err := bot.Send(msg); err != nil {
							log.Printf("Error sending bot list: %v", err)
						}
					case "add":
						newBotToken := update.Message.CommandArguments()
						if len(newBotToken) > 0 {
							newbot, err := tgbotapi.NewBotAPI(newBotToken)
							if err != nil {
								log.Panic(err)
							}
							config.BotTokens = append(config.BotTokens, BotToken{
								Name:  newbot.Self.UserName,
								Token: newBotToken,
							})
							activeBots = append(activeBots, newbot.Self.UserName)
							go startClientBot(logstream, newBotToken)
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Bot added: @"+newbot.Self.UserName)
							if _, err := bot.Send(msg); err != nil {
								log.Printf("Error sending command arguments: %v", err)
							}
						} else {
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Provide bot token.")
							if _, err := bot.Send(msg); err != nil {
								log.Printf("Error sending no arguments message: %v", err)
							}
						}
					default:
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command.")
						if _, err := bot.Send(msg); err != nil {
							log.Printf("Error sending unknown command response: %v", err)
						}
					}
				} else {
					// Handle other messages
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Hehehehe")
					if _, err := bot.Send(msg); err != nil {
						log.Printf("Error sending message: %v", err)
					}
				}
			}
		}
	}
}

func startClientBot(logstream chan string, botToken string) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)
	botName := bot.Self.UserName
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	for {
		updates := bot.GetUpdatesChan(u)

		for update := range updates {
			if update.Message != nil {
				var userRequest UserRequest
				userRequest.UserMessage = update.Message.Text
				userRequest.UserID = fmt.Sprintf("%d", update.Message.From.ID)

				// Log user replica
				if userRequest.UserMessage != "" {
					logstream <- userRequest.UserID + ":" + userRequest.UserMessage
				}
				// Clarify user, provide appropriate thread
				threadID, exists := threadStore.GetThread(botName, userRequest.UserID)

				if !exists || threadID == "" {
					// Create a new thread
					threadID, err = CreateThread()

					if err != nil {
						log.Printf("failed to create thread: %v", err)
						continue
					}
					threadStore.SaveThread(botName, userRequest.UserID, threadID)
					ExportThreads(threadsFileName)
					log.Printf("Thread %s created\n", threadID)
				} else {
					log.Printf("Thread %s found\n", threadID)
				}

				response, err := HandleMessage(userRequest.UserMessage, threadID, assistantID)
				if err != nil {
					log.Printf("Error handling message: %v", err)
					continue
				}
				// Store response
				logstream <- response
				// Response
				_, err = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, response))
				if err != nil {
					log.Printf("Error writing message: %v", err)
					continue
				}
			}
		}
	}
}

func fileWriter(input chan string, filename string) {
	var data string
	for {
		data = <-input
		file, err := os.OpenFile("./"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			fmt.Printf("File process failed, %s\n", err.Error())
		}
		if data != "" {
			file.WriteString(data + "\n")
		}
		file.Close()
	}

}

func readConfig() Config {
	//Init config
	var appConfig Config
	file, err := os.Open("./config.json")
	if err != nil {
		log.Println("Error occured while reading config; Should be config.json with apiKey, assistandID and port")
		os.Exit(1)
	}
	defer file.Close()

	data, _ := io.ReadAll(file)

	err = json.Unmarshal(data, &appConfig)
	if err != nil {
		log.Println("Incorrect configuration; Should be config.json with apiKey, assistandID and port")
		os.Exit(1)
	}
	return appConfig
}
func writeConfig(config Config) error {
	file, err := os.Create("./config.json")
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to write config to file: %w", err)
	}

	return nil
}

func ExportThreads(filename string) error {
	threadStore.Lock()
	defer threadStore.Unlock()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	return encoder.Encode(threadStore)
}

func ImportThreads(filename string) error {
	threadStore.Lock()
	defer threadStore.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist, initialize empty threads
			threadStore = NewThreadStore()
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(&threadStore)
}
