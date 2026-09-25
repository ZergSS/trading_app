package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
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

	instruments  map[string][]string
	names        map[string]string
	volatilities map[string]float64
}

type SearchResult struct {
	Ticker   string
	Name     string
	Category string
}

func NewApp(cfg *config.Config) *App {
	a := &App{
		tviewApp:     tview.NewApplication(),
		cfg:          cfg,
		pages:        tview.NewPages(),
		tables:       make(map[string]*tview.Table),
		instruments:  make(map[string][]string),
		names:        make(map[string]string),
		volatilities: make(map[string]float64),
		categories:   []string{"coins", "stocks"},
		activeIndex:  0,
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

	a.loadSavedInstruments()
	a.buildUI()
	return a
}

func (a *App) loadSavedInstruments() {
	items, err := a.storage.LoadInstruments()
	if err == nil && len(items) > 0 {
		for _, it := range items {
			cat := it.Type
			if !containsString(a.categories, cat) {
				continue
			}
			a.instruments[cat] = append(a.instruments[cat], it.Ticker)
			if it.Name != "" {
				a.names[it.Ticker] = it.Name
			}
		}
		return
	}

	defaultCoins := []string{
		"BTCUSDT",
		"ETHUSDT",
		"SOLUSDT",
		"XRPUSDT",
		"DOGEUSDT",
		"LTCUSDT",
		"NEARUSDT",
		"ARUSDT",
		"HYPEUSDT",
		"BCHUSDT",
		"ARBUSDT",
		"1000PEPEUSDT",
		"UNIUSDT",
	}

	defaultStocks := []struct {
		Ticker string
		Name   string
	}{
		{Ticker: "SBER", Name: "Сбербанк"},
		{Ticker: "GAZP", Name: "Газпром"},
		{Ticker: "LKOH", Name: "Лукойл"},
		{Ticker: "GMKN", Name: "ГМК Норильский Никель"},
		{Ticker: "NVTK", Name: "Новатэк"},
		{Ticker: "ROSN", Name: "Роснефть"},
		{Ticker: "YDEX", Name: "Яндекс"},
		{Ticker: "MTSS", Name: "МТС"},
		{Ticker: "TATN", Name: "Татнефть"},
		{Ticker: "VTBR", Name: "ВТБ"},
		{Ticker: "SNGS", Name: "Сургутнефтегаз"},
		{Ticker: "CHMF", Name: "Северсталь"},
		{Ticker: "PLZL", Name: "Полюс"},
		{Ticker: "ALRS", Name: "Алроса"},
		{Ticker: "MOEX", Name: "Московская биржа"},
		{Ticker: "T", Name: "Т-Банк"},
		{Ticker: "AFKS", Name: "АФК Система"},
		{Ticker: "X5", Name: "КЦ ИКС 5"},
		{Ticker: "HYDR", Name: "РусГидро"},
		{Ticker: "RUAL", Name: "Русал"},
		{Ticker: "SNGSP", Name: "Сургутнефтегаз преф."},
		{Ticker: "MAGN", Name: "ММК"},
		{Ticker: "IRAO", Name: "Интер РАО"},
		{Ticker: "PHOR", Name: "ФосАгро"},
		{Ticker: "TATNP", Name: "Татнефть преф."},
		{Ticker: "LSRG", Name: "Группа ЛСР"},
		{Ticker: "NLMK", Name: "НЛМК"},
		{Ticker: "RASP", Name: "Распадская"},
		{Ticker: "SIBN", Name: "Газпром нефть"},
	}

	for _, coin := range defaultCoins {
		a.instruments["coins"] = append(a.instruments["coins"], coin)
		a.names[coin] = coin
		_ = a.storage.SaveInstrument(storage.Instrument{
			Ticker: coin,
			Name:   coin,
			Type:   "coins",
		})
	}

	for _, stock := range defaultStocks {
		a.instruments["stocks"] = append(a.instruments["stocks"], stock.Ticker)
		a.names[stock.Ticker] = stock.Name
		_ = a.storage.SaveInstrument(storage.Instrument{
			Ticker: stock.Ticker,
			Name:   stock.Name,
			Type:   "stocks",
		})
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
	if err := a.finamClient.Connect(); err != nil {
		a.showError(fmt.Sprintf("Не удалось подключиться: %v", err))
	} else {
		a.status.SetText("Готово")
	}

	for cat, list := range a.instruments {
		for _, ticker := range list {
			c, t := cat, ticker
			go a.updateInstrument(c, t)
		}
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
	// Ровно две колонки (по 50% на Монеты и Акции)
	grid := tview.NewGrid().
		SetRows(0, 1).
		SetColumns(0, 0)

	a.tables["coins"] = newTable("Монеты")
	a.tables["stocks"] = newTable("Акции")

	grid.AddItem(a.tables["coins"], 0, 0, 1, 1, 0, 0, true)
	grid.AddItem(a.tables["stocks"], 0, 1, 1, 1, 0, 0, true)

	a.search = tview.NewInputField().
		SetLabel("Добавить тикер [coins]: ").
		SetPlaceholder("например BTC или SBER").
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

	// Растягиваем нижнюю панель на 2 колонки
	grid.AddItem(bottom, 1, 0, 1, 2, 0, 0, false)

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

			case tcell.KeyRight:
				if a.search.HasFocus() && a.search.GetText() == "" {
					a.activateNext()
					return nil
				}
				if !a.search.HasFocus() {
					a.activateNext()
					return nil
				}

			case tcell.KeyLeft:
				if a.search.HasFocus() && a.search.GetText() == "" {
					a.activatePrev()
					return nil
				}
				if !a.search.HasFocus() {
					a.activatePrev()
					return nil
				}
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

func (a *App) refreshTable(category string) {
	table := a.tables[category]
	if table == nil {
		return
	}

	type item struct {
		ticker string
		name   string
		vol    float64
	}

	var items []item
	for _, t := range a.instruments[category] {
		items = append(items, item{
			ticker: t,
			name:   a.names[t],
			vol:    a.volatilities[t],
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].vol > items[j].vol
	})

	rowCount := table.GetRowCount()
	for r := rowCount - 1; r > 0; r-- {
		table.RemoveRow(r)
	}

	for i, it := range items {
		row := i + 1
		displayName := it.ticker
		if it.name != "" && it.name != it.ticker {
			displayName = fmt.Sprintf("%s (%s)", it.name, it.ticker)
		}
		setTableRow(table, row, displayName, it.vol)
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
		a.volatilities[ticker] = vol
		a.refreshTable(category)
		a.status.SetText("Готово")
	})
}

func (a *App) performSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}

	if a.searchCancel != nil {
		a.searchCancel()
	}

	searchCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	a.searchCancel = cancel

	startedAt := time.Now()
	a.status.SetText("Поиск...")
	a.searchTimer.SetText("Поиск: 0.0s")

	stopTimer := make(chan struct{})

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopTimer:
				return
			case <-searchCtx.Done():
				return
			case <-ticker.C:
				delta := time.Since(startedAt)
				a.tviewApp.QueueUpdate(func() {
					a.searchTimer.SetText(fmt.Sprintf("Поиск: %.1fs", delta.Seconds()))
				})
			}
		}
	}()

	currentCategory := a.categories[a.activeIndex]

	go func(q string, cat string) {
		defer func() {
			select {
			case <-stopTimer:
			default:
				close(stopTimer)
			}
			cancel()
		}()

		var results []SearchResult
		var searchErr error

		if cat == "coins" {
			upper := strings.ToUpper(q)
			var candidates []string
			if strings.HasSuffix(upper, "USDT") && len(upper) > 4 {
				candidates = []string{upper}
			} else {
				candidates = []string{upper + "USDT", upper}
			}

			for _, sym := range candidates {
				candles, err := a.bybitClient.GetCandles(searchCtx, sym, a.cfg.VolatilityPeriod)
				if err == nil && len(candles) > 0 {
					results = append(results, SearchResult{
						Ticker:   sym,
						Name:     sym,
						Category: "coins",
					})
					break
				} else if err != nil {
					searchErr = err
				}
			}
		} else {
			symbols, err := a.finamClient.Search(searchCtx, q)
			if err != nil {
				searchErr = err
			} else {
				for _, s := range symbols {
					targetCat := s.Category
					if targetCat == "" {
						targetCat = cat
					}
					results = append(results, SearchResult{
						Ticker:   s.Ticker,
						Name:     s.Name,
						Category: targetCat,
					})
				}
			}
		}

		log.Printf("Поиск '%s' (категория '%s'): results=%d, err=%v", q, cat, len(results), searchErr)

		a.tviewApp.QueueUpdateDraw(func() {
			a.status.SetText("Готово")
			a.searchTimer.SetText("Поиск: 0.0s")

			if len(results) == 0 {
				errMsg := fmt.Sprintf("Ничего не найдено для '%s'", q)
				if searchErr != nil {
					errMsg = fmt.Sprintf("Ошибка: %v", searchErr)
				}
				a.displayErrorModal(errMsg)
				return
			}

			a.showSearchResults(results)
		})
	}(query, currentCategory)
}

func (a *App) showSearchResults(results []SearchResult) {
	list := tview.NewList().
		ShowSecondaryText(true)
	list.SetBorder(true).
		SetTitle(" Результаты поиска (Enter — добавить, Esc — отмена) ").
		SetTitleColor(tcell.ColorYellow).
		SetBorderColor(tcell.ColorLightCyan).
		SetBackgroundColor(tcell.NewRGBColor(24, 28, 40))

	for _, r := range results {
		r := r
		title := r.Ticker
		if r.Name != "" && r.Name != r.Ticker {
			title = fmt.Sprintf("%s (%s)", r.Name, r.Ticker)
		}
		desc := fmt.Sprintf("Категория: %s", r.Category)
		list.AddItem(title, desc, 0, func() {
			a.names[r.Ticker] = r.Name
			a.addInstrument(r.Ticker, r.Name, r.Category)
			a.search.SetText("")
			a.pages.RemovePage("search_modal")
			a.tviewApp.SetFocus(a.search)
		})
	}

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			a.pages.RemovePage("search_modal")
			a.tviewApp.SetFocus(a.search)
			return nil
		}
		return event
	})

	modalGrid := tview.NewGrid().
		SetColumns(-1, 72, -1).
		SetRows(-1, 15, -1).
		AddItem(list, 1, 1, 1, 1, 0, 0, true)

	if a.pages.HasPage("search_modal") {
		a.pages.RemovePage("search_modal")
	}

	a.pages.AddPage("search_modal", modalGrid, true, true)
	a.tviewApp.SetFocus(list)
}

func (a *App) displayErrorModal(msg string) {
	modal := tview.NewModal().
		SetText(msg).
		SetTextColor(tcell.ColorBlack).
		SetBackgroundColor(tcell.ColorWhiteSmoke).
		SetButtonTextColor(tcell.ColorWhite).
		SetButtonBackgroundColor(tcell.ColorDarkBlue).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.RemovePage("error_modal")
			a.tviewApp.SetFocus(a.search)
		})

	if a.pages.HasPage("error_modal") {
		a.pages.RemovePage("error_modal")
	}

	a.pages.AddPage("error_modal", modal, true, true)
	a.tviewApp.SetFocus(modal)
}

func (a *App) showError(msg string) {
	a.tviewApp.QueueUpdateDraw(func() {
		a.displayErrorModal(msg)
	})
}

func (a *App) addInstrument(ticker, name, category string) {
	if _, ok := a.tables[category]; !ok {
		return
	}
	for _, t := range a.instruments[category] {
		if t == ticker {
			return
		}
	}
	a.instruments[category] = append(a.instruments[category], ticker)
	a.names[ticker] = name
	_ = a.storage.SaveInstrument(storage.Instrument{Ticker: ticker, Name: name, Type: category})
	go a.updateInstrument(category, ticker)
}
