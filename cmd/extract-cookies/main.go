// Package main implements a CLI tool to extract browser cookies for social media platforms.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/codeGROOVE-dev/sociopath/pkg/auth"
)

// platform defines a supported social media platform for cookie extraction.
type platform struct {
	envMap    map[string]string
	name      string
	domain    string
	envPrefix string
	cookies   []string
}

var platforms = []platform{
	{
		name:      "instagram",
		domain:    "instagram.com",
		envPrefix: "INSTAGRAM",
		cookies:   []string{"sessionid", "csrftoken"},
		envMap:    map[string]string{"sessionid": "SESSIONID", "csrftoken": "CSRFTOKEN"},
	},
	{
		name:      "linkedin",
		domain:    "linkedin.com",
		envPrefix: "LINKEDIN",
		cookies:   []string{"li_at", "JSESSIONID", "lidc", "bcookie"},
		envMap:    map[string]string{"li_at": "LI_AT", "JSESSIONID": "JSESSIONID", "lidc": "LIDC", "bcookie": "BCOOKIE"},
	},
	{
		name:      "tiktok",
		domain:    "tiktok.com",
		envPrefix: "TIKTOK",
		cookies:   []string{"sessionid"},
		envMap:    map[string]string{"sessionid": "SESSIONID"},
	},
	{
		name:      "twitter",
		domain:    "x.com",
		envPrefix: "TWITTER",
		cookies:   []string{"auth_token", "ct0", "kdt", "twid", "att"},
		envMap:    map[string]string{"auth_token": "AUTH_TOKEN", "ct0": "CT0", "kdt": "KDT", "twid": "TWID", "att": "ATT"},
	},
	{
		name:      "vkontakte",
		domain:    "vk.com",
		envPrefix: "VK",
		cookies:   []string{"remixsid"},
		envMap:    map[string]string{"remixsid": "REMIXSID"},
	},
	{
		name:      "weibo",
		domain:    "weibo.com",
		envPrefix: "WEIBO",
		cookies:   []string{"SUB", "SUBP"},
		envMap:    map[string]string{"SUB": "SUB", "SUBP": "SUBP"},
	},
}

func main() {
	listPlatforms := flag.Bool("list", false, "List supported platforms and their required cookies")
	platformFilter := flag.String("platform", "", "Filter to specific platform")
	flag.Parse()

	if *listPlatforms {
		printPlatformList()
		return
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	results := extractCookies(ctx, logger, *platformFilter)

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No cookies found. Make sure you're logged into the social media platforms in your browser.")
		os.Exit(1)
	}

	printResults(results)
}

func printPlatformList() {
	fmt.Println("Supported platforms and required cookies:")
	fmt.Println()

	for _, p := range platforms {
		fmt.Printf("  %s:\n", p.name)
		fmt.Printf("    Domain:  %s\n", p.domain)
		fmt.Printf("    Cookies: %s\n", strings.Join(p.cookies, ", "))
		fmt.Printf("    Env prefix: %s_<COOKIE_NAME>\n", p.envPrefix)
		fmt.Println()
	}
}

type cookieResult struct {
	cookies  map[string]string
	platform platform
}

func extractCookies(ctx context.Context, logger *slog.Logger, platformFilter string) []cookieResult {
	var results []cookieResult
	source := auth.NewBrowserSource(logger)

	for _, p := range platforms {
		if platformFilter != "" && p.name != platformFilter {
			continue
		}

		cookies, err := source.Cookies(ctx, p.name)
		if err != nil {
			logger.Debug("failed to read cookies", "platform", p.name, "error", err)
			continue
		}

		if len(cookies) > 0 {
			results = append(results, cookieResult{platform: p, cookies: cookies})
		}
	}

	return results
}

func printResults(results []cookieResult) {
	for i, r := range results {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("# %s\n", strings.ToUpper(r.platform.name))

		// Sort cookie names for consistent output
		names := make([]string, 0, len(r.cookies))
		for name := range r.cookies {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			envSuffix := strings.ToUpper(name)
			if r.platform.envMap != nil {
				if mapped, ok := r.platform.envMap[name]; ok {
					envSuffix = mapped
				}
			}
			envName := fmt.Sprintf("%s_%s", r.platform.envPrefix, envSuffix)
			fmt.Printf("%s=%s\n", envName, r.cookies[name])
		}
	}
}
