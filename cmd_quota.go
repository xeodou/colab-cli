package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

func runQuota(args []string) error {
	jsonOutput := hasFlag(args, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := getToken(ctx)
	if err != nil {
		return err
	}

	client := NewColabClient(tok.AccessToken)
	info, err := client.GetUserInfo(ctx)
	if err != nil {
		return err
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Subscription tier
	fmt.Printf("Tier:           %s\n", info.Tier)

	// Paid CCU
	if info.PaidBalance > 0 {
		fmt.Printf("Paid CCUs:      %.1f compute units\n", info.PaidBalance)
	}
	if info.BurnRate > 0 {
		fmt.Printf("Burn rate:      %.1f CCU/hr\n", info.BurnRate)
		if info.PaidBalance > 0 {
			mins := (info.PaidBalance / info.BurnRate) * 60
			fmt.Printf("Paid time left: %s\n", formatMinutes(mins))
		}
	}

	// Free quota
	if info.FreeRemaining > 0 {
		fmt.Printf("Free CCUs:      %.1f compute units\n", info.FreeRemaining)
		if info.BurnRate > 0 {
			mins := (info.FreeRemaining / info.BurnRate) * 60
			fmt.Printf("Free time left: ~%s (at current burn rate)\n", formatMinutes(mins))
		} else {
			// Estimate with typical T4 rate (~1.96 CCU/hr)
			mins := (info.FreeRemaining / 1.96) * 60
			fmt.Printf("Free time left: ~%s (estimated for T4)\n", formatMinutes(mins))
		}
	}
	if !info.FreeRefillTime.IsZero() {
		fmt.Printf("Free refill:    %s\n", info.FreeRefillTime.Format("2006-01-02 15:04 MST"))
	}

	// Active runtimes
	if info.ActiveRuntimes > 0 {
		fmt.Printf("Active:         %d runtime(s)\n", info.ActiveRuntimes)
	}

	// Eligible GPUs
	if len(info.EligibleGPUs) > 0 {
		fmt.Printf("GPUs:           %s\n", join(info.EligibleGPUs))
	}
	if len(info.EligibleTPUs) > 0 {
		fmt.Printf("TPUs:           %s\n", join(info.EligibleTPUs))
	}
	unavailable := append(info.IneligibleGPUs, info.IneligibleTPUs...)
	if len(unavailable) > 0 {
		fmt.Printf("Unavailable:    %s (upgrade tier)\n", join(unavailable))
	}

	return nil
}

func formatMinutes(mins float64) string {
	if mins < 1 {
		return "<1 min"
	}
	h := int(math.Floor(mins / 60))
	m := int(math.Mod(mins, 60))
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func join(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
