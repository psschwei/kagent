package cli

import (
	"context"
	"fmt"
	"slices"

	"github.com/abiosoft/ishell/v2"
	autogen_client "github.com/kagent-dev/kagent/go/autogen/client"
	"github.com/kagent-dev/kagent/go/cli/internal/config"
	"github.com/kagent-dev/kagent/go/cli/internal/ws"
	"github.com/spf13/pflag"
	"golang.org/x/exp/rand"
)

const (
	sessionCreateNew = "[New Session]"
)

func generateRandomString(prefix string, length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	return prefix + string(b), nil
}

func ChatCmd(c *ishell.Context) {

	verbose := false
	flagSet := pflag.NewFlagSet(c.RawArgs[0], pflag.ContinueOnError)
	flagSet.BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	if err := flagSet.Parse(c.Args); err != nil {
		c.Printf("Failed to parse flags: %v\n", err)
		return
	}

	cfg, err := config.Get()
	if err != nil {
		fmt.Printf("Failed to get config: %v\n", err)
		return
	}

	client := autogen_client.New(cfg.APIURL, cfg.WSURL)

	var team *autogen_client.Team
	if len(flagSet.Args()) > 0 {
		teamName := flagSet.Args()[0]
		team, err = client.GetTeam(cfg.UserID, teamName)
		if err != nil {
			c.Println(err)
			return
		}
	} else {
		// Get the teams based on the input + userID
		teams, err := client.ListTeams(cfg.UserID)
		if err != nil {
			c.Println(err)
			return
		}

		if len(teams) == 0 {
			c.Println("No teams found, please create one via the web UI or CRD before chatting.")
			return
		}

		teamNames := make([]string, len(teams))
		for i, team := range teams {
			if team.Component.Label == nil {
				continue
			}
			teamNames[i] = *team.Component.Label
		}

		selectedTeamIdx := c.MultiChoice(teamNames, "Select an agent:")
		team = teams[selectedTeamIdx]
	}

	sessions, err := client.ListSessions(cfg.UserID)
	if err != nil {
		c.Println(err)
		return
	}

	existingSessions := slices.Collect(Filter(slices.Values(sessions), func(session *autogen_client.Session) bool {
		return session.TeamID == team.Id
	}))

	existingSessionNames := slices.Collect(Map(slices.Values(existingSessions), func(session *autogen_client.Session) string {
		return session.Name
	}))

	// Add the new session option to the beginning of the list
	existingSessionNames = append([]string{sessionCreateNew}, existingSessionNames...)
	selectedSessionIdx := c.MultiChoice(existingSessionNames, "Select a session:")

	var session *autogen_client.Session
	if selectedSessionIdx == 0 {
		c.ShowPrompt(false)
		c.Print("Enter a session name: ")
		sessionName, err := c.ReadLineErr()
		if err != nil {
			c.Println(err)
			c.ShowPrompt(true)
			return
		}
		c.ShowPrompt(true)
		session, err = client.CreateSession(&autogen_client.CreateSession{
			UserID: cfg.UserID,
			Name:   sessionName,
			TeamID: team.Id,
		})
		if err != nil {
			c.Println(err)
			return
		}
	} else {
		session = existingSessions[selectedSessionIdx-1]
	}

	promptStr := BoldGreen(fmt.Sprintf("%s--%s> ", *team.Component.Label, session.Name))
	c.SetPrompt(promptStr)

	run, err := client.CreateRun(&autogen_client.CreateRunRequest{
		SessionID: session.ID,
		UserID:    session.UserID,
	})
	if err != nil {
		c.Println(err)
		return
	}

	wsConfig := ws.DefaultConfig()
	wsConfig.Verbose = verbose
	wsClient, err := ws.NewClient(cfg.WSURL, run.ID, wsConfig)
	if err != nil {
		c.Println(err)
		return
	}
	c.ShowPrompt(false)
	c.Print("Enter a task: ")
	task, err := c.ReadLineErr()
	if err != nil {
		c.Println(err)
		c.ShowPrompt(true)
		return
	}
	c.ShowPrompt(true)
	wsClient.StartInteractive(context.Background(), c, team, task)
}
