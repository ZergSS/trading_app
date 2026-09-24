package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"finam-dashboard/internal/bybit"
	"finam-dashboard/internal/config"
	"finam-dashboard/internal/finam"
	"finam-dashboard/internal/storage"
	"finam-dashboard/internal/volatility"
)

type App struct {
	tviewApp *tview.Application
	cfg      *config.Config
	pages    *tview.Pages

	finamClient *finam.Client
	bybitClient *bybit.Client
	storage     *storage.Storage

	tables map[string]*tview.Table
	search *tview.InputField
	status *tview.TextView

	instruments map[string][]string
}

type SearchResult struct {
	Ticker   string
	Name     string
	Category string
}

func NewApp(cfg *config.Config) *App {
	a := &App{
		tviewApp: tview.NewApplication(),
		cfg:      cfg,
		pages:    tview.NewPages(),
		tables:   make(map[string]*tview.Table),
		instruments: map[string][]string{
			"coins":  {"BTCUSDT", "ETHUSDT"},
			"stocks": {"SBER", "GAZP"},
			"bonds":  {"SU26238RMFS5"},
		},
	}

	a.finamClient = finam.NewClient(cfg.FinamToken)
	a.bybitClient = bybit.NewClient()

	var err error
	a.storage, err = storage.New(cfg.DBPath)
	if err != nil {
		panic(fmt.Sprintf("Ошибка создания хранилища: %v", err))
	}
	if err := a.storage.Init(); err != nil {
		panic(fmt.Sprintf("Ошибка инициализации хранилища: %v", err))
	}

	a.buildUI()
	return a
}

func (a *App) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.finamClient.Connect(ctx); err != nil {
		a.showError(fmt.Sprintf("Не удалось подключиться к Finam API: %v", err))
	} else {
		a.status.SetText("Finam: OK")
	}

	a.refreshData()
	go a.autoRefresh()

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
		SetLabel("Поиск: ").
		SetPlaceholder("тикер или название").
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				query := strings.TrimSpace(a.search.GetText())
				if query != "" {
					a.performSearch(query)
				}
			}
		})

	a.status = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignLeft)

	bottom := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.search, 0, 3, true).
		AddItem(a.status, 0, 1, false)

	grid.AddItem(bottom, 1, 0, 1, 4, 0, 0, false)

	a.pages.AddPage("main", grid, true, true)

	a.tviewApp.SetRoot(a.pages, true).
		SetFocus(a.search).
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyF5 {
				a.refreshData()
			}
			return event
		})
}

func (a *App) refreshData() {
	a.status.SetText("Обновление...")
	for category, tickers := range a.instruments {
		for _, ticker := range tickers {
			go a.updateInstrument(category, ticker)
		}
	}
}

func (a *App) updateInstrument(category, ticker string) {
	ctx := context.Background()
	var candles []volatility.Candle
	var err error

	switch category {
	case "coins":
		candles, err = a.bybitClient.GetCandles(ctx, ticker, a.cfg.VolatilityPeriod)
	default:
		candles, err = a.finamClient.GetCandles(ctx, ticker, a.cfg.VolatilityPeriod)
	}

	if err != nil {
		a.showError(fmt.Sprintf("Ошибка при загрузке %s: %v", ticker, err))
		return
	}

	vol := volatility.Calculate(candles)
	lastClose := candles[len(candles)-1].Close

	_ = a.storage.SaveVolatility(ticker, vol, lastClose)

	tview.QueueUpdateDraw(func() {
		table := a.tables[category]
		if table == nil {
			table = a.tables["other"]
		}
		updateTableRow(table, ticker, vol)
		a.status.SetText("Готово")
	})
}

func (a *App) autoRefresh() {
	interval := time.Duration(a.cfg.UpdateInterval) * time.Second
	for {
		time.Sleep(interval)
		a.refreshData()
	}
}

func (a *App) performSearch(query string) {
	var results []SearchResult

	// Проверяем Bybit (если похоже на тикер USDT)
	if strings.Contains(strings.ToUpper(query), "USDT") {
		candles, err := a.bybitClient.GetCandles(context.Background(), strings.ToUpper(query), a.cfg.VolatilityPeriod)
		if err == nil && len(candles) > 0 {
			results = append(results, SearchResult{
				Ticker:   strings.ToUpper(query),
				Name:     query,
				Category: "coins",
			})
		}
	}

	// Проверяем Finam
	symbols, err := a.finamClient.Search(context.Background(), query)
	if err == nil {
		for _, s := range symbols {
			results = append(results, SearchResult{
				Ticker:   s.Ticker,
				Name:     s.Name,
				Category: s.Category,
			})
		}
	}

	if len(results) == 0 {
		a.showError("Ничего не найдено")
		return
	}

	a.showSearchResults(results)
}

func (a *App) showSearchResults(results []SearchResult) {
	list := tview.NewList()
	for _, r := range results {
		r := r
		list.AddItem(
			fmt.Sprintf("%s (%s) — %s", r.Ticker, r.Name, r.Category),
			"", 0,
			func() {
				a.addInstrument(r.Ticker, r.Category)
				a.pages.RemovePage("search")
			},
		)
	}
	list.AddItem("Отмена", "", 0, func() {
		a.pages.RemovePage("search")
	})

	modal := tview.NewFlex().AddItem(list, 0, 1, true)
	modal.SetBorder(true).SetTitle("Результаты поиска")

	a.pages.AddPage("search", modal, true, true)
}

func (a *App) addInstrument(ticker, category string) {
	if _, ok := a.tables[category]; !ok {
		category = "other"
	}
	a.instruments[category] = append(a.instruments[category], ticker)
	go a.updateInstrument(category, ticker)
}

func (a *App) showError(msg string) {
	tview.QueueUpdateDraw(func() {
		modal := tview.NewModal().
			SetText(msg).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				a.pages.RemovePage("error")
			})
		a.pages.AddPage("error", modal, true, true)
	})
}
