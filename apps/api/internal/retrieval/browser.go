package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/chromedp"
)

type browserRung struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
}

func newBrowserRung() (*browserRung, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserDataDir("/tmp/retrieval-browser"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return nil, fmt.Errorf("retrieval: launch browser rung: %w", err)
	}

	return &browserRung{allocCtx: allocCtx, allocCancel: allocCancel}, nil
}

func (b *browserRung) Fetch(ctx context.Context, url string) (string, error) {
	tabCtx, tabCancel := chromedp.NewContext(b.allocCtx)
	defer tabCancel()

	deadlineCtx, cancel := context.WithTimeout(tabCtx, 45*time.Second)
	defer cancel()

	var html string
	err := chromedp.Run(deadlineCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return "", fmt.Errorf("browser: fetch %s: %w", url, err)
	}
	return html, nil
}

func (b *browserRung) Close() {
	if b.allocCancel != nil {
		b.allocCancel()
	}
	slog.Info("retrieval: browser rung closed")
}
