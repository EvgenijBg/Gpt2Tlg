package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var apiKey string

type CreateThreadResponse struct {
	ID string `json:"id"`
}

type CreateMessageResponse struct {
	ID string `json:"id"`
}

type CreateRunResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type RetrieveRunResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"last_error"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TextContent struct {
	Type string `json:"type"`
	Text struct {
		Value       string        `json:"value"`
		Annotations []interface{} `json:"annotations"`
	} `json:"text"`
}
type ListMessagesResponse struct {
	Object  string        `json:"object"`
	Data    []RespMessage `json:"data"`
	FirstID string        `json:"first_id"`
	LastID  string        `json:"last_id"`
	HasMore bool          `json:"has_more"`
}

type RespMessage struct {
	ID          string                 `json:"id"`
	Object      string                 `json:"object"`
	CreatedAt   int64                  `json:"created_at"`
	AssistantID string                 `json:"assistant_id,omitempty"`
	ThreadID    string                 `json:"thread_id"`
	RunID       string                 `json:"run_id,omitempty"`
	Role        string                 `json:"role"`
	Content     []TextContent          `json:"content"`
	Attachments []interface{}          `json:"attachments"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func HandleMessage(message string, threadID string, assistandID string) (string, error) {
	// Send the user's message to the thread
	fmt.Printf("Handling with msg `%s` thrID `%s`\n", message, threadID)
	var err = createMessage(threadID, message)
	if err != nil {
		return "", fmt.Errorf("failed to create message: %v", err)
	}

	// Run the assistant
	runID, err := createRun(threadID, assistandID)
	if err != nil {
		return "", fmt.Errorf("failed to create run: %v", err)
	}
	fmt.Printf("Run %s created\n", runID)

	// Wait for the run to complete
	var counter int = 0
	for {
		status, err := retrieveRun(threadID, runID)
		counter += 1
		if counter%31 == 0 {
			fmt.Printf("Current status:%s\n;", status)
			counter = 0
		}
		if err != nil || status == "failed" {
			return "", fmt.Errorf("failed to retrieve run: %v", err)
		}
		if status == "completed" || status == "complete" {
			break
		}

	}
	// Retrieve the messages from the thread
	messages, err := listMessages(threadID)
	if err != nil {
		return "", fmt.Errorf("failed to list messages: %v", err)
	}
	fmt.Println("Done retrieving")
	// Return the content of the last message
	for i := 0; i < len(messages); i++ {
		fmt.Println(messages[i].Content)
	}
	return messages[0].Content[0].Text.Value, nil
}

func CreateThread() (string, error) {
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/threads", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "error", err
	}
	defer resp.Body.Close()
	var createThreadResponse CreateThreadResponse
	if err := json.NewDecoder(resp.Body).Decode(&createThreadResponse); err != nil {
		return "error", err
	}

	return createThreadResponse.ID, nil
}

func createMessage(threadID, content string) error {
	message := Message{
		Role:    "user",
		Content: content,
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var createMessageResponse CreateMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&createMessageResponse); err != nil {
		return err
	}

	return nil
}

func createRun(threadID string, assistantID string) (string, error) {
	fmt.Printf("Creating run for threadID `%s`\n", threadID)
	run := map[string]string{
		"assistant_id": assistantID,
	}
	body, err := json.Marshal(run)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs", threadID), bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var createRunResponse CreateRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&createRunResponse); err != nil {
		return "", err
	}

	return createRunResponse.ID, nil
}

func retrieveRun(threadID string, runID string) (string, error) {
	fmt.Printf("Handling with runID `%s` thrID `%s`\n", runID, threadID)
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs/%s", threadID, runID), nil)
	if err != nil {
		fmt.Printf("Error, %v\n", err)
		return "Error", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error, %v\n", err)
		return "Error", err
	}
	var retrieveRunResponse RetrieveRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&retrieveRunResponse); err != nil {
		fmt.Printf("Error, %v\n", err)
		return "Error", err
	}
	fmt.Println(retrieveRunResponse.Error)
	return retrieveRunResponse.Status, nil
}

func listMessages(threadID string) ([]RespMessage, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// data, _ := io.ReadAll(resp.Body)
	// fmt.Println("%s\n", string(data))
	var listMessagesResponse ListMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&listMessagesResponse); err != nil {
		return nil, err
	}

	return listMessagesResponse.Data, nil
}
