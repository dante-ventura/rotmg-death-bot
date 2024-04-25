package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Players              []string `json:"players"`
	DiscordWebhook       string   `json:"discordWebhook"`
	DiscordAvatarUrl     string   `json:"discordAvatarUrl"`
	DiscordDeathMessages []string `json:"discordDeathMessages"`
	TimeBetweenRequest   int      `json:"timeBetweenRequest"`
	RequestUserAgent     string   `json:"requestUserAgent"`
}

type PlayerDeath struct {
	username string
	date     time.Time
	class    string
	baseFame string
	killedBy string
}

var globalConfig Config
var latestDeaths map[string]time.Time

func parseConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(data, &globalConfig)
	if err != nil {
		panic(err)
	}
}

func randomDeathMessage(user string) string {
	randIdx := rand.Intn(len(globalConfig.DiscordDeathMessages))
	msg := globalConfig.DiscordDeathMessages[randIdx]
	return strings.Replace(msg, "%username%", user, -1)
}

func sendDiscordDeathMsg(playerDeath PlayerDeath) {
	webhookBodyBytes := []byte(`
		{
			"username": "` + playerDeath.killedBy + `",
			"avatar_url": "` + globalConfig.DiscordAvatarUrl + `",
			"embeds": [
				{
					"title": "` + randomDeathMessage(playerDeath.username) + `",
					"url": "https://www.realmeye.com/graveyard-of-player/` + playerDeath.username + `",
					"color": 16777215,
					"fields": [
						{
							"name": "Time",
							"value": "` + playerDeath.date.Format("2006-01-02 15:04:05 UTC") + `",
							"inline": false
						},
						{
							"name": "Class",
							"value": "` + playerDeath.class + `",
							"inline": true
						},
						{
							"name": "Base Fame",
							"value": "` + playerDeath.baseFame + `",
							"inline": true
						}
					]
				}
			]
		}
	`)

	client := http.Client{}
	req, err := http.NewRequest(
		"POST",
		globalConfig.DiscordWebhook,
		bytes.NewBuffer(webhookBodyBytes),
	)
	if err != nil {
		fmt.Println("Error forming discord webhook request:", err)
		return
	}

	defer req.Body.Close()
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error requesting discord webhook:", err)
		return
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		fmt.Println("Error requesting discord webhook, got response code:", resp.StatusCode)
		return
	}

	defer resp.Body.Close()
}

func setupDeathMap() {
	latestDeaths = make(map[string]time.Time)
	for _, plrName := range globalConfig.Players {
		latestDeaths[plrName] = time.Date(0001, 01, 01, 00, 00, 00, 00, time.UTC)
	}
}

func runIter() {
	client := http.Client{}
	for _, plrName := range globalConfig.Players {
		time.Sleep(time.Duration(globalConfig.TimeBetweenRequest) * time.Millisecond)

		req, err := http.NewRequest(
			"GET",
			"https://www.realmeye.com/graveyard-of-player/"+plrName,
			nil,
		)
		if err != nil {
			fmt.Println("Error forming realmeye graveyard request:", err)
			continue
		}

		req.Header.Add("User-Agent", globalConfig.RequestUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error requesting realmeye graveyard:", err)
			continue
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading http request body:", err)
			continue
		}

		regTbody := regexp.MustCompile("<tbody>(.*?)</tbody>")
		matchesTbody := regTbody.FindStringSubmatch(string(body))
		if len(matchesTbody) < 2 {
			fmt.Println("Error finding <tbody> via regex:")
			continue
		}
		tbody := &matchesTbody[1]

		regRow := regexp.MustCompile("<tr>(.*?)</tr>")
		matchesRow := regRow.FindAllString(*tbody, -1)
		if len(matchesRow) < 10 {
			fmt.Println("Error finding <tr> via regex:")
			continue
		}
		rowBody := &matchesRow[0]

		regData := regexp.MustCompile("<td>(.*?)</td>")
		matchesData := regData.FindAllStringSubmatch(*rowBody, -1)
		if len(matchesData) < 1 {
			fmt.Println("Error finding <td> via regex:")
			continue
		}
		latestDeathDateStr := &matchesData[0][1]

		latestDeathTime, err := time.Parse("2006-01-02T15:04:05Z", *latestDeathDateStr)
		if err != nil {
			fmt.Println("Error parsing time:", err)
			continue
		}

		if latestDeaths[plrName].Year() == 0001 {
			latestDeaths[plrName] = latestDeathTime
			continue
		}

		if latestDeathTime.Equal(latestDeaths[plrName]) ||
			latestDeathTime.Before(latestDeaths[plrName]) {
			continue
		}

		latestDeaths[plrName] = latestDeathTime
		playerDeath := PlayerDeath{
			username: plrName,
			date:     latestDeathTime,
			class:    matchesData[2][1],
			baseFame: matchesData[4][1],
			killedBy: matchesData[9][1],
		}
		sendDiscordDeathMsg(playerDeath)
	}
}

func run() {
	for {
		runIter()
	}
}

func main() {
	configPath := "config.json"
	if len(os.Args) == 2 {
		configPath = os.Args[1]
	}
	parseConfig(configPath)
	setupDeathMap()
	run()
}
