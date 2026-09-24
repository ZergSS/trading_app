package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newTable(title string) *tview.Table {
	table := tview.NewTable()
	table.SetBorder(true).SetTitle(title)
	table.SetFixed(1, 0)
	table.SetSelectable(true, false)

	// Заголовки
	table.SetCell(0, 0, tview.NewTableCell("Тикер").SetExpansion(1))
	table.SetCell(0, 1, tview.NewTableCell("% волатильности").SetAlign(tview.AlignRight))
	table.SetCell(0, 2, tview.NewTableCell("Торг").SetAlign(tview.AlignCenter))

	return table
}

func tradeText(vol float64) (string, tcell.Color) {
	switch {
	case vol < 1.0:
		return "не торгуем", tcell.ColorRed
	case vol < 1.5:
		return "можно попытаться", tcell.ColorOrange
	case vol < 3.0:
		return "торгуем точно", tcell.ColorGreen
	default:
		return "торгуем осторожно", tcell.ColorPurple
	}
}

func setTableRow(table *tview.Table, row int, ticker string, vol float64) {
	table.SetCell(row, 0, tview.NewTableCell(ticker).SetExpansion(1))
	table.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf("%.2f%%", vol)).SetAlign(tview.AlignRight))
	text, color := tradeText(vol)
	table.SetCell(row, 2, tview.NewTableCell(text).SetTextColor(color).SetAlign(tview.AlignCenter))
}
