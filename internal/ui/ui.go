package ui

// C:\_Projects_Go\AcousticMerge\internal\ui\ui.go
// Package: ui
// Назначение: Цветной UI, help, парсинг аргументов, автопути D:\→C:\, 2 прогресс-бара (PASS1/PASS2).

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const Version = "v0.3.0"

type OrderBy string

const (
	OrderByName  OrderBy = "name"
	OrderByMTime OrderBy = "mtime"
)

type Config struct {
	Src           string
	Out           string
	GainPct       float64
	Order         OrderBy
	StrictFormat  bool
	Resample      int
	NormalizeDB   float64
	DoNormalize   bool
	CrossfadeMS   int
	DryRun        bool
	BarWidth      int
	NoColor       bool
	NoEmoji       bool
	MergeNow      bool
	ShowOnlyHelp  bool
}

// ---------- Цвета/эмодзи/логгеры ----------

var (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cCyan   = "\x1b[36m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cGray   = "\x1b[90m"
)

func col(noColor bool, s, color string) string {
	if noColor {
		return s
	}
	return color + s + cReset
}

func emoji(noEmoji bool, s string) string {
	if noEmoji {
		return ""
	}
	return s + " "
}

type UIAPI struct {
	LogInfo  func(format string, a ...any)
	LogOK    func(format string, a ...any)
	LogWarn  func(format string, a ...any)
	LogErr   func(format string, a ...any)
	PrintKV  func(k, v string)
	PrintBar func(label string, cur, total int)
	EndBar   func()
	Banner   func(title string)
}

var API UIAPI // инициализируется в ParseArgsAndSetup

// ---------- Help/баннер ----------

func autodetectBaseDrive() string {
	if _, err := os.Stat(`D:\`); err == nil {
		return `D:\`
	}
	return `C:\`
}

func defaultPaths() (src, out string) {
	base := autodetectBaseDrive()
	src = filepath.Join(base, "DataSound_Temp", "AcousticMerge", "Raw")
	out = filepath.Join(base, "DataSound_Temp", "AcousticMerge", "Result", "merged.wav")
	return
}

func showHelp(noColor bool) {
	base := autodetectBaseDrive()
	defSrc := filepath.Join(base, "DataSound_Temp", "AcousticMerge", "Raw")
	defOut := filepath.Join(base, "DataSound_Temp", "AcousticMerge", "Result", "merged.wav")

	title := "AcousticMerge — offline WAV merger (AcousticLog companion tool)"
	sep := strings.Repeat("─", len(title)+2)
	fmt.Printf("%s\n%s %s\n%s\n",
		col(noColor, sep, cCyan),
		col(noColor, "▶", cCyan), col(noColor, title, cBold),
		col(noColor, sep, cCyan),
	)

	fmt.Println(col(noColor, "Быстрый старт:", cCyan))
	fmt.Println("  AcousticMerge /merge                 → склеить по умолчанию (из Raw в Result)")
	fmt.Println("  AcousticMerge /merge --gain-pct 150 → склеить и усилить ×1.5")
	fmt.Println()

	fmt.Println(col(noColor, "Параметры:", cCyan))
	fmt.Printf("  --src <путь>         Папка с WAV-файлами (рекурсивный сбор). По умолчанию: %s\n", defSrc)
	fmt.Printf("  --out <путь>         Итоговый WAV. По умолчанию: %s\n", defOut)
	fmt.Println("  --gain-pct <число>   Усиление в процентах (100=как есть, 150=×1.5, 200=×2.0)")
	fmt.Println("  --normalize <дБ>     Пик-нормализация до уровня (дБFS), напр. -1.0")
	fmt.Println("  --order name|mtime   Порядок: по имени или по времени изменения")
	fmt.Println("  --crossfade-ms <мс>  Лёгкий фейд на стыках (0=выкл)")
	fmt.Println("  --dry-run            Только проверка (без записи файла)")
	fmt.Println("  --bar-width <N>      Ширина прогресс-бара (80 по умолчанию)")
	fmt.Println("  --no-color           Отключить цвет")
	fmt.Println("  --no-emoji           Отключить эмодзи")
	fmt.Println("  --merge              Алиас для быстрого старта с путями по умолчанию")
	fmt.Println("  --help, /?, -?       Показать справку и выйти")
	fmt.Println()

	fmt.Println(col(noColor, "Примеры:", cCyan))
	fmt.Println("  AcousticMerge                       → показать справку (пустой запуск)")
	fmt.Println("  AcousticMerge /merge                → запустить с путями по умолчанию")
	fmt.Println("  AcousticMerge /merge --gain-pct 120 → усиление ×1.2")
	fmt.Println("  AcousticMerge --order mtime /merge  → сортировка по времени + склейка")
	fmt.Println("  AcousticMerge --dry-run --src X     → только проверит и покажет сводку")
	fmt.Println()

	fmt.Println(col(noColor, "📂 Пути по умолчанию:", cCyan))
	fmt.Println("  • Если диск D: найден → D:\\DataSound_Temp\\AcousticMerge\\Raw / Result")
	fmt.Println("  • Иначе используется  → C:\\DataSound_Temp\\AcousticMerge\\Raw / Result")
	fmt.Println("  • Папки создаются при первом запуске автоматически.")
	fmt.Println()
	fmt.Println(col(noColor, "(C) Andrey Koval, 2025. AcousticMerge — дополнение к AcousticLog.", cGray))
}

// ---------- Парсинг/алиасы и настройка UI ----------

func ParseArgsAndSetup() (*Config, bool) {
	cfg := &Config{}

	mergeNow := false
	showHelpOnly := false
	cleanArgs := make([]string, 0, len(os.Args)-1)
	for _, a := range os.Args[1:] {
		switch {
		case a == "/?" || a == "-?" || strings.EqualFold(a, "--help"):
			showHelpOnly = true
		case strings.EqualFold(a, "/merge") || strings.EqualFold(a, "--merge"):
			mergeNow = true
		default:
			cleanArgs = append(cleanArgs, a)
		}
	}

	defSrc, defOut := defaultPaths()

	var (
		flagSrc         string
		flagOut         string
		flagGainPct     float64
		flagOrder       string
		flagStrict      bool
		flagResample    int
		flagNormalizeDB float64
		flagCrossfadeMS int
		flagDryRun      bool
		flagNoColor     bool
		flagBarW        int
		flagNoEmoji     bool
	)

	flag.StringVar(&flagSrc, "src", defSrc, "Папка с WAV-файлами (рекурсивный сбор)")
	flag.StringVar(&flagOut, "out", defOut, "Путь к итоговому файлу (если занят — merged_1.wav и т.д.)")
	flag.Float64Var(&flagGainPct, "gain-pct", 100, "Усиление в процентах: 100=как есть, 150=×1.5, 200=×2.0")
	flag.StringVar(&flagOrder, "order", string(OrderByName), "Порядок: name|mtime")
	flag.BoolVar(&flagStrict, "strict-format", true, "Требовать одинаковый формат (PCM16/SR/каналы). Иначе ошибка")
	flag.IntVar(&flagResample, "resample", 0, "Привести sample rate к указанному (не реализовано)")
	flag.Float64Var(&flagNormalizeDB, "normalize", math.NaN(), "Пик-нормализация до уровня (дБFS), напр. -1.0")
	flag.IntVar(&flagCrossfadeMS, "crossfade-ms", 0, "Кроссфейд на стыках (мс). 0 = без кроссфейда")
	flag.BoolVar(&flagDryRun, "dry-run", false, "Только проверить и вывести сводку (без записи)")

	flag.BoolVar(&flagNoColor, "no-color", false, "Отключить цветной вывод")
	flag.IntVar(&flagBarW, "bar-width", 80, "Ширина прогресс-бара (символов)")
	flag.BoolVar(&flagNoEmoji, "no-emoji", false, "Отключить эмодзи")

	if err := flag.CommandLine.Parse(cleanArgs); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	// Help по требованию или пустой запуск — показать и выйти
	if showHelpOnly || (!mergeNow && len(cleanArgs) == 0) {
		showHelp(flagNoColor)
		return &Config{ShowOnlyHelp: true}, true
	}

	// Сформировать конфиг
	cfg.Src = flagSrc
	cfg.Out = flagOut
	cfg.GainPct = flagGainPct
	cfg.Order = OrderBy(strings.ToLower(flagOrder))
	cfg.StrictFormat = flagStrict
	cfg.Resample = flagResample
	cfg.NormalizeDB = flagNormalizeDB
	cfg.DoNormalize = !math.IsNaN(flagNormalizeDB)
	cfg.CrossfadeMS = flagCrossfadeMS
	cfg.DryRun = flagDryRun
	cfg.BarWidth = flagBarW
	cfg.NoColor = flagNoColor
	cfg.NoEmoji = flagNoEmoji
	cfg.MergeNow = mergeNow

	// Настроить UI
	API = makeUI(flagNoColor, flagNoEmoji, flagBarW)
	API.Banner("AcousticMerge")

	return cfg, false
}

func makeUI(noColor, noEmoji bool, barW int) UIAPI {
	logInfo := func(format string, a ...any) {
		fmt.Printf("%s%s\n", emoji(noEmoji, "ℹ️"), fmt.Sprintf(format, a...))
	}
	logOK := func(format string, a ...any) {
		fmt.Printf("%s%s\n", emoji(noEmoji, "✅"), col(noColor, fmt.Sprintf(format, a...), cGreen))
	}
	logWarn := func(format string, a ...any) {
		fmt.Printf("%s%s\n", emoji(noEmoji, "⚠️"), col(noColor, fmt.Sprintf(format, a...), cYellow))
	}
	logErr := func(format string, a ...any) {
		fmt.Printf("%s%s\n", emoji(noEmoji, "🛑"), fmt.Sprintf(format, a...))
	}
	printKV := func(k, v string) {
		const pad = 12
		if len(k) < pad { k = k + strings.Repeat(" ", pad-len(k)) }
		fmt.Printf("   %s %s\n", col(noColor, k, cGray), v)
	}
	printBar := func(label string, cur, total int) {
		if total <= 0 { return }
		if barW < 10 { barW = 10 }
		percent := float64(cur) / float64(total)
		if percent < 0 { percent = 0 }
		if percent > 1 { percent = 1 }
		width := barW
		fill := int(math.Round(percent * float64(width)))
		if fill > width { fill = width }

		full := "█"
		empty := "░"
		green := cGreen
		reset := cReset
		if noColor { green = ""; reset = "" }

		bar := strings.Repeat(full, fill) + strings.Repeat(empty, width-fill)
		if !noColor && fill > 0 {
			bar = green + strings.Repeat(full, fill) + reset + strings.Repeat(empty, width-fill)
		}
		prefix := "🟩 "
		if noEmoji { prefix = "" }
		fmt.Printf("\r%s%-12s [%s] %6.2f%% (%d/%d)", prefix, label, bar, percent*100.0, cur, total)
	}
	endBar := func() { fmt.Print("\n") }
	banner := func(title string) {
		sep := strings.Repeat("─", len(title)+2)
		fmt.Printf("%s\n%s %s\n%s\n",
			col(noColor, sep, cCyan),
			col(noColor, "▶", cCyan), col(noColor, title, cBold),
			col(noColor, sep, cCyan),
		)
	}

	return UIAPI{
		LogInfo: logInfo, LogOK: logOK, LogWarn: logWarn, LogErr: logErr,
		PrintKV: printKV, PrintBar: printBar, EndBar: endBar, Banner: banner,
	}
}
