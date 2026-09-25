package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"trade_info/internal/bybit"
	"trade_info/internal/config"
	"trade_info/internal/finam"
	"trade_info/internal/storage"
	"trade_info/internal/volatility"
)

type App struct {
	tviewApp *tview.Application
	cfg      *config.Config
	pages    *tview.Pages

	finamClient *finam.Client
	bybitClient *bybit.Client
	storage     *storage.Storage

	tables       map[string]*tview.Table
	search       *tview.InputField
	status       *tview.TextView
	searchTimer  *tview.TextView
	searchCancel context.CancelFunc

	categories  []string
	activeIndex int

	instruments map[string][]string
	rows        map[string]map[string]int
}

type SearchResult struct {
	Ticker   string
	Name     string
	Category string
}

func NewApp(cfg *config.Config) *App {
	a := &App{
		tviewApp:    tview.NewApplication(),
		cfg:         cfg,
		pages:       tview.NewPages(),
		tables:      make(map[string]*tview.Table),
		instruments: map[string][]string{},
		rows:        make(map[string]map[string]int),
		categories:  []string{"coins", "stocks", "bonds", "other"},
		activeIndex: 0,
	}

	var err error
	a.finamClient, err = finam.NewClient(cfg.FinamToken)
	if err != nil {
		log.Printf("finam client init error: %v", err)
	}
	a.bybitClient = bybit.NewClient()

	a.storage, err = storage.New(cfg.DBPath)
	if err != nil {
		panic(fmt.Sprintf("Ошибка создания хранилища: %v", err))
	}
	if err := a.storage.Init(); err != nil {
		panic(fmt.Sprintf("Ошибка инициализации хранилища: %v", err))
	}

	a.loadSavedInstruments()
	a.buildUI()
	return a
}

func (a *App) loadSavedInstruments() {
	items, err := a.storage.LoadInstruments()
	if err != nil {
		return
	}
	for _, it := range items {
		cat := it.Type
		if !containsString(a.categories, cat) {
			cat = "other"
		}
		a.instruments[cat] = append(a.instruments[cat], it.Ticker)
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (a *App) Run() error {
	// Connect без передачи ctx
	if err := a.finamClient.Connect(); err != nil {
		a.showError(fmt.Sprintf("Не удалось подключиться к Finam API: %v", err))
	} else {
		a.status.SetText("Finam: OK")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		if a.searchCancel != nil {
			a.searchCancel()
		}
		a.tviewApp.Stop()
	}()

	return a.tviewApp.Run()
}

func (a *App) buildUI() {
	grid := tview.NewGrid().SetRows(0, 1).SetColumns(0, 0, 0, 0)

	a.tables["coins"] = newTable("Монеты")
	a.tables["stocks"] = newTable("Акции")
	a.tables["bonds"] = newTable("Облигации")
	a.tables["other"] = newTable("Прочее")

	grid.AddItem(a.tables["coins"], 0, 0, 1, 1, 0, 0, true)
	grid.AddItem(a.tables["stocks"], 0, 1, 1, 1, 0, 0, true)
	grid.AddItem(a.tables["bonds"], 0, 2, 1, 1, 0, 0, true)
	grid.AddItem(a.tables["other"], 0, 3, 1, 1, 0, 0, true)

	a.search = tview.NewInputField().
		SetLabel("Добавить тикер: ").
		SetPlaceholder("например SBER@MISX").
		SetFieldTextColor(tcell.ColorWhite).
		SetFieldBackgroundColor(tcell.NewRGBColor(12, 28, 38)).
		SetPlaceholderTextColor(tcell.ColorDarkGray).
		SetLabelColor(tcell.ColorLightGreen).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				query := strings.TrimSpace(a.search.GetText())
				if query != "" {
					a.performSearch(query)
				}
			}
		})

	a.status = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignLeft)
	a.status.SetText("Готово")
	a.status.SetTextColor(tcell.ColorLightSkyBlue)
	a.status.SetBackgroundColor(tcell.NewRGBColor(12, 28, 38))

	a.searchTimer = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	a.searchTimer.SetText("Поиск: 0.0s")
	a.searchTimer.SetTextColor(tcell.ColorLightGreen)
	a.searchTimer.SetBackgroundColor(tcell.NewRGBColor(12, 28, 38))

	bottom := tview.NewFlex().SetDirection(tview.FlexColumn)
	bottom.SetBackgroundColor(tcell.NewRGBColor(12, 28, 38))
	bottom.AddItem(a.search, 0, 5, true)
	bottom.AddItem(a.status, 14, 0, false)
	bottom.AddItem(a.searchTimer, 16, 0, false)

	grid.AddItem(bottom, 1, 0, 1, 4, 0, 0, false)

	a.pages.AddPage("main", grid, true, true)

	a.activate(a.activeIndex)

	a.tviewApp.SetRoot(a.pages, true).
		SetFocus(a.search).
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyCtrlC:
				if a.searchCancel != nil {
					a.searchCancel()
				}
				a.tviewApp.Stop()
				return nil
			case tcell.KeyF5:
				a.activateNext()
				return nil
			}
			return event
		})
}

func (a *App) activate(i int) {
	if i < 0 {
		i = len(a.categories) - 1
	}
	if i >= len(a.categories) {
		i = 0
	}
	a.activeIndex = i

	for idx, cat := range a.categories {
		color := tcell.ColorGray
		if idx == i {
			color = tcell.ColorGreen
		}
		a.tables[cat].SetBorderColor(color)
	}

	a.search.SetLabel("Добавить тикер [" + a.categories[i] + "]: ")
}

func (a *App) activateNext() { a.activate(a.activeIndex + 1) }

func (a *App) activatePrev() { a.activate(a.activeIndex - 1) }

func (a *App) refreshData() {
	a.status.SetText("Обновление отключено")
}

func (a *App) updateInstrument(category, ticker string) {
	ctx := context.Background()
	var candles []volatility.Candle
	var err error

	switch category {
	case "coins":
		// Bybit клиент сразу возвращает []volatility.Candle
		candles, err = a.bybitClient.GetCandles(ctx, ticker, a.cfg.VolatilityPeriod)
	default:
		var finamCandles []finam.Candle
		finamCandles, err = a.finamClient.GetCandles(ctx, ticker, a.cfg.VolatilityPeriod)
		if err == nil {
			for _, c := range finamCandles {
				candles = append(candles, volatility.Candle{
					High:  c.High,
					Low:   c.Low,
					Close: c.Close,
				})
			}
		}
	}

	if err != nil {
		a.showError(fmt.Sprintf("Ошибка при загрузке %s: %v", ticker, err))
		return
	}

	if len(candles) == 0 {
		a.showError(fmt.Sprintf("Нет данных по инструменту %s", ticker))
		return
	}

	vol := volatility.Calculate(candles)
	lastClose := candles[len(candles)-1].Close

	_ = a.storage.SaveVolatility(ticker, vol, lastClose)

	a.tviewApp.QueueUpdateDraw(func() {
		table := a.tables[category]
		if table == nil {
			table = a.tables["other"]
			category = "other"
		}
		if a.rows[category] == nil {
			a.rows[category] = make(map[string]int)
		}
		row, ok := a.rows[category][ticker]
		if !ok {
			row = table.GetRowCount()
			a.rows[category][ticker] = row
		}
		setTableRow(table, row, ticker, vol)
		a.status.SetText("Готово")
	})
}

func (a *App) autoRefresh() {
	return
}

func (a *App) performSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}

	searchCtx, cancel := context.WithCancel(context.Background())
	a.searchCancel = cancel

	startedAt := time.Now()
	a.status.SetText("Поиск...")
	a.searchTimer.SetText("Поиск: 0.0s")

	done := make(chan struct{})

	// Таймер теперь не перегружает UI и корректно освобождает поток
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-searchCtx.Done():
				return
			case <-ticker.C:
				delta := time.Since(startedAt)
				a.tviewApp.QueueUpdateDraw(func() {
					a.searchTimer.SetText(fmt.Sprintf("Поиск: %.1fs", delta.Seconds()))
				})
			}
		}
	}()

	go func(q string) {
		defer func() {
			select {
			case <-done:
			default:
				close(done)
			}
			a.searchCancel = nil
		}()

		var results []SearchResult

		if a.categories[a.activeIndex] == "coins" {
			candles, err := a.bybitClient.GetCandles(searchCtx, strings.ToUpper(q), a.cfg.VolatilityPeriod)
			if err == nil && len(candles) > 0 {
				results = append(results, SearchResult{
					Ticker:   strings.ToUpper(q),
					Name:     q,
					Category: "coins",
				})
			}
		} else {
			symbols, err := a.finamClient.Search(searchCtx, q)
			if err == nil {
				for _, s := range symbols {
					cat := mapFinamTypeToCategory(s.Type)
					results = append(results, SearchResult{
						Ticker:   s.Ticker,
						Name:     s.Name,
						Category: cat,
					})
				}
			}
		}

		// Если поиск уже был отменён пользователем — не трогаем UI
		if searchCtx.Err() != nil {
			return
		}

		a.tviewApp.QueueUpdateDraw(func() {
			a.status.SetText("Готово")
			a.searchTimer.SetText("Поиск: 0.0s")
			if len(results) == 0 {
				a.showError("Ничего не найдено")
				return
			}
			a.showSearchResults(results)
		})
	}(query)
}

func mapFinamTypeToCategory(finamType string) string {
	switch strings.ToUpper(finamType) {
	case "STOCK":
		return "stocks"
	case "BOND":
		return "bonds"
	default:
		return "other"
	}
}

func (a *App) showSearchResults(results []SearchResult) {
	list := tview.NewList()
	list.SetMainTextColor(tcell.ColorBlack)
	list.SetSecondaryTextColor(tcell.ColorBlack)
	list.SetBackgroundColor(tcell.ColorWhite)
	list.SetBorder(true)
	list.SetTitle("Результаты поиска")
	list.SetRect(10, 4, 100, 18)
	for _, r := range results {
		r := r
		list.AddItem(
			fmt.Sprintf("%s (%s) — %s", r.Ticker, r.Name, r.Category),
			"", 0,
			func() {
				a.addInstrument(r.Ticker, r.Category)
				a.search.SetText("")
				a.pages.RemovePage("search")
				a.tviewApp.SetFocus(a.search)
			},
		)
	}
	list.AddItem("Отмена", "", 0, func() {
		a.search.SetText("")
		a.pages.RemovePage("search")
		a.tviewApp.SetFocus(a.search)
	})

	a.pages.AddPage("search", list, true, true)
}

func (a *App) addInstrument(ticker, category string) {
	if _, ok := a.tables[category]; !ok {
		category = "other"
	}
	for _, t := range a.instruments[category] {
		if t == ticker {
			return
		}
	}
	a.instruments[category] = append(a.instruments[category], ticker)
	_ = a.storage.SaveInstrument(storage.Instrument{Ticker: ticker, Name: ticker, Type: category})
	go a.updateInstrument(category, ticker)
}

func (a *App) showError(msg string) {
	a.tviewApp.QueueUpdateDraw(func() {
		modal := tview.NewModal().
			SetText(msg).
			SetTextColor(tcell.ColorBlack).
			SetBackgroundColor(tcell.ColorWhite).
			SetButtonTextColor(tcell.ColorBlack).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				a.pages.RemovePage("error")
				a.tviewApp.SetFocus(a.search)
			})
		a.pages.AddPage("error", modal, true, true)
	})
}
