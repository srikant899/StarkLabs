package main

import (
    "encoding/json"
    "fmt"
    "io"
    "math"
    "net/http"
    "os"
    "sort"
    "strings"
    "time"
)

type topMoversResponse struct {
    TopGainers []struct {
        Ticker           string `json:"ticker"`
        Price            string `json:"price"`
        ChangePercentage string `json:"change_percentage"`
        Volume           string `json:"volume"`
    } `json:"top_gainers"`
    MostActivelyTraded []struct {
        Ticker           string `json:"ticker"`
        Price            string `json:"price"`
        ChangePercentage string `json:"change_percentage"`
        Volume           string `json:"volume"`
    } `json:"most_actively_traded"`
}

type dailyAdjustedResponse struct {
    MetaData map[string]string               `json:"Meta Data"`
    Series   map[string]map[string]string    `json:"Time Series (Daily)"`
}

type SymbolReport struct {
    Symbol          string        `json:"symbol"`
    Rank            int           `json:"rank"`
    CurrentPrice    float64       `json:"currentPrice"`
    DayChangePct    float64       `json:"dayChangePct"`
    Volume          float64       `json:"volume"`
    MomentumScore   float64       `json:"momentumScore"`
    QualityScore    float64       `json:"qualityScore"`
    RiskBand        string        `json:"riskBand"`
    Thesis          string        `json:"thesis"`
    Targets         []TargetRow   `json:"targets"`
}

type TargetRow struct {
    Horizon         string  `json:"horizon"`
    TargetPrice     float64 `json:"targetPrice"`
    UpsidePct       float64 `json:"upsidePct"`
    StopLoss        float64 `json:"stopLoss"`
    Confidence      float64 `json:"confidence"`
}

type ReportFile struct {
    GeneratedAt     time.Time      `json:"generatedAt"`
    Provider        string         `json:"provider"`
    Universe        string         `json:"universe"`
    Stocks          []SymbolReport `json:"stocks"`
    Notes           []string       `json:"notes"`
}

type dailyBar struct {
    Date   time.Time
    Close  float64
    Volume float64
}

func main() {
    apiKey := os.Getenv("ALPHAVANTAGE_API_KEY")
    if apiKey == "" {
        writeMockReport()
        fmt.Println("ALPHAVANTAGE_API_KEY not set; generated mock top20 report")
        return
    }

    report, err := buildRealReport(apiKey)
    if err != nil {
        fmt.Fprintf(os.Stderr, "real report failed, falling back to mock: %v\n", err)
        writeMockReport()
        return
    }

    if err := writeReport(report); err != nil {
        panic(err)
    }
    fmt.Printf("generated %d stock reports\n", len(report.Stocks))
}

func buildRealReport(apiKey string) (ReportFile, error) {
    movers, err := fetchTopMovers(apiKey)
    if err != nil {
        return ReportFile{}, err
    }

    symbols := make([]string, 0, 20)
    seen := map[string]bool{}
    for _, x := range movers.TopGainers {
        if len(symbols) == 20 { break }
        if !seen[x.Ticker] {
            seen[x.Ticker] = true
            symbols = append(symbols, x.Ticker)
        }
    }
    for _, x := range movers.MostActivelyTraded {
        if len(symbols) == 20 { break }
        if !seen[x.Ticker] {
            seen[x.Ticker] = true
            symbols = append(symbols, x.Ticker)
        }
    }

    reports := make([]SymbolReport, 0, len(symbols))
    for _, sym := range symbols {
        bars, err := fetchDailyAdjusted(apiKey, sym)
        if err != nil || len(bars) < 80 {
            continue
        }
        rep := analyzeSymbol(sym, bars)
        reports = append(reports, rep)
        time.Sleep(14 * time.Second)
    }

    sort.Slice(reports, func(i, j int) bool {
        return reports[i].QualityScore > reports[j].QualityScore
    })
    for i := range reports {
        reports[i].Rank = i + 1
    }

    if len(reports) > 20 {
        reports = reports[:20]
    }

    return ReportFile{
        GeneratedAt: time.Now().UTC(),
        Provider:    "alphavantage",
        Universe:    "Top gainers + most actively traded (deduped, top 20)",
        Stocks:      reports,
        Notes: []string{
            "Targets are model-based estimates, not guarantees.",
            "Free-tier mode uses one top-movers request plus up to 20 daily-history requests.",
            "Realtime US options data is not included in this free workflow.",
        },
    }, nil
}

func fetchTopMovers(apiKey string) (topMoversResponse, error) {
    u := fmt.Sprintf("https://www.alphavantage.co/query?function=TOP_GAINERS_LOSERS&apikey=%s", apiKey)
    var out topMoversResponse
    if err := getJSON(u, &out); err != nil { return out, err }
    if len(out.TopGainers) == 0 && len(out.MostActivelyTraded) == 0 {
        return out, fmt.Errorf("empty top movers response")
    }
    return out, nil
}

func fetchDailyAdjusted(apiKey, symbol string) ([]dailyBar, error) {
    u := fmt.Sprintf("https://www.alphavantage.co/query?function=TIME_SERIES_DAILY_ADJUSTED&outputsize=compact&symbol=%s&apikey=%s", symbol, apiKey)
    var raw dailyAdjustedResponse
    if err := getJSON(u, &raw); err != nil { return nil, err }
    if len(raw.Series) == 0 { return nil, fmt.Errorf("empty history for %s", symbol) }

    bars := make([]dailyBar, 0, len(raw.Series))
    for ds, fields := range raw.Series {
        d, err := time.Parse("2006-01-02", ds)
        if err != nil { continue }
        closePx := atof(fields["5. adjusted close"])
        if closePx == 0 { closePx = atof(fields["4. close"]) }
        bars = append(bars, dailyBar{Date: d, Close: closePx, Volume: atof(fields["6. volume"])})
    }
    sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
    return bars, nil
}

func analyzeSymbol(symbol string, bars []dailyBar) SymbolReport {
    latest := bars[len(bars)-1]
    prev := bars[len(bars)-2]
    dayChangePct := pct(latest.Close, prev.Close)

    r20 := pct(latest.Close, bars[maxInt(0, len(bars)-21)].Close)
    r60 := pct(latest.Close, bars[maxInt(0, len(bars)-61)].Close)
    r120 := pct(latest.Close, bars[maxInt(0, len(bars)-121)].Close)

    vol20 := realizedVol(bars, 20)
    vol60 := realizedVol(bars, 60)
    avgVol20 := avgVolume(bars, 20)
    volRatio := safeDiv(latest.Volume, max(avgVol20, 1))

    momentum := clamp(50+r20*0.7+r60*0.45+r120*0.2, 1, 99)
    quality := clamp(50+r20*0.5+r60*0.5+(volRatio-1.0)*10-(vol20*100)*0.25, 1, 99)
    riskBand := riskLabel(vol20, vol60)

    annualDrift := clamp((r60/100.0)*4.0+(r120/100.0)*2.0, -0.35, 0.60)
    annualVol := clamp((vol20+vol60)/2.0, 0.15, 0.95)
    bias := clamp((momentum-50)/100.0, -0.25, 0.35)

    targets := []TargetRow{
        projectTarget(latest.Close, annualDrift, annualVol, bias, 5.0/252.0, "1 week"),
        projectTarget(latest.Close, annualDrift, annualVol, bias, 21.0/252.0, "1 month"),
        projectTarget(latest.Close, annualDrift, annualVol, bias, 63.0/252.0, "3 months"),
        projectTarget(latest.Close, annualDrift, annualVol, bias, 126.0/252.0, "6 months"),
        projectTarget(latest.Close, annualDrift, annualVol, bias, 252.0/252.0, "12 months"),
    }

    thesis := fmt.Sprintf("20d %.1f%%, 60d %.1f%%, vol ratio %.2fx, realized vol %.2f%%", r20, r60, volRatio, vol20*100)
    return SymbolReport{
        Symbol:        symbol,
        CurrentPrice:  round(latest.Close),
        DayChangePct:  round(dayChangePct),
        Volume:        latest.Volume,
        MomentumScore: round(momentum),
        QualityScore:  round(quality),
        RiskBand:      riskBand,
        Thesis:        thesis,
        Targets:       targets,
    }
}

func projectTarget(price, annualDrift, annualVol, bias, t float64, horizon string) TargetRow {
    mu := annualDrift + bias
    sigma := annualVol
    expected := price * math.Exp((mu-0.5*sigma*sigma)*t)
    band := price * sigma * math.Sqrt(t) * 0.8
    target := round(expected + band*0.35)
    stop := round(math.Max(price-band*0.85, price*0.70))
    upside := round((target/price - 1) * 100)
    confidence := round(clamp(72-sigma*40+t*8+bias*30, 35, 92))
    return TargetRow{Horizon: horizon, TargetPrice: target, UpsidePct: upside, StopLoss: stop, Confidence: confidence}
}

func writeMockReport() {
    report := ReportFile{
        GeneratedAt: time.Now().UTC(),
        Provider:    "mock",
        Universe:    "Mock top 20 stocks",
        Stocks: []SymbolReport{
            {Symbol: "NVDA", Rank: 1, CurrentPrice: 142.4, DayChangePct: 3.04, Volume: 89000000, MomentumScore: 91.2, QualityScore: 88.4, RiskBand: "high", Thesis: "AI leader with strong trend and volume expansion", Targets: []TargetRow{{Horizon: "1 week", TargetPrice: 147.8, UpsidePct: 3.79, StopLoss: 135.1, Confidence: 74}, {Horizon: "1 month", TargetPrice: 154.9, UpsidePct: 8.78, StopLoss: 131.4, Confidence: 72}, {Horizon: "3 months", TargetPrice: 167.0, UpsidePct: 17.28, StopLoss: 126.8, Confidence: 69}, {Horizon: "6 months", TargetPrice: 178.3, UpsidePct: 25.21, StopLoss: 122.4, Confidence: 66}, {Horizon: "12 months", TargetPrice: 198.7, UpsidePct: 39.54, StopLoss: 114.0, Confidence: 62}}},
            {Symbol: "META", Rank: 2, CurrentPrice: 611.0, DayChangePct: 1.26, Volume: 23200000, MomentumScore: 82.0, QualityScore: 80.3, RiskBand: "medium", Thesis: "Large-cap trend intact with moderate volatility", Targets: []TargetRow{{Horizon: "1 week", TargetPrice: 623.0, UpsidePct: 1.96, StopLoss: 586.0, Confidence: 78}, {Horizon: "1 month", TargetPrice: 642.0, UpsidePct: 5.07, StopLoss: 572.0, Confidence: 76}, {Horizon: "3 months", TargetPrice: 671.0, UpsidePct: 9.82, StopLoss: 548.0, Confidence: 73}, {Horizon: "6 months", TargetPrice: 704.0, UpsidePct: 15.22, StopLoss: 526.0, Confidence: 70}, {Horizon: "12 months", TargetPrice: 758.0, UpsidePct: 24.06, StopLoss: 500.0, Confidence: 66}}},
        },
        Notes: []string{"Mock report shown because API key is missing.", "Add ALPHAVANTAGE_API_KEY as a repository secret for live daily stock targets."},
    }
    _ = writeReport(report)
}

func writeReport(report ReportFile) error {
    if err := os.MkdirAll("docs", 0o755); err != nil { return err }
    f, err := os.Create("docs/top20.json")
    if err != nil { return err }
    defer f.Close()
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    return enc.Encode(report)
}

func getJSON(url string, v any) error {
    client := &http.Client{Timeout: 25 * time.Second}
    req, _ := http.NewRequest(http.MethodGet, url, nil)
    res, err := client.Do(req)
    if err != nil { return err }
    defer res.Body.Close()
    b, _ := io.ReadAll(res.Body)
    if strings.Contains(string(b), "Note") || strings.Contains(string(b), "Information") || strings.Contains(string(b), "Error Message") {
        return fmt.Errorf("api response: %s", string(b))
    }
    return json.Unmarshal(b, v)
}

func realizedVol(bars []dailyBar, n int) float64 {
    if len(bars) < n+1 { return 0.35 }
    start := len(bars) - n
    rets := make([]float64, 0, n)
    for i := start; i < len(bars); i++ {
        if i == 0 { continue }
        r := math.Log(bars[i].Close / bars[i-1].Close)
        rets = append(rets, r)
    }
    m := 0.0
    for _, x := range rets { m += x }
    m /= float64(len(rets))
    v := 0.0
    for _, x := range rets { d := x - m; v += d * d }
    v /= math.Max(1, float64(len(rets)-1))
    return math.Sqrt(v) * math.Sqrt(252)
}

func avgVolume(bars []dailyBar, n int) float64 {
    if len(bars) < n { n = len(bars) }
    s := 0.0
    for _, b := range bars[len(bars)-n:] { s += b.Volume }
    return safeDiv(s, float64(n))
}

func riskLabel(v20, v60 float64) string {
    v := (v20 + v60) / 2
    switch {
    case v < 0.28:
        return "low"
    case v < 0.48:
        return "medium"
    default:
        return "high"
    }
}

func pct(curr, prev float64) float64 { return safeDiv(curr-prev, prev) * 100 }
func round(v float64) float64 { return math.Round(v*100) / 100 }
func clamp(v, lo, hi float64) float64 { if v < lo { return lo }; if v > hi { return hi }; return v }
func safeDiv(a, b float64) float64 { if b == 0 { return 0 }; return a / b }
func atof(s string) float64 { var v float64; fmt.Sscanf(s, "%f", &v); return v }
func max(a, b float64) float64 { if a > b { return a }; return b }
func maxInt(a, b int) int { if a > b { return a }; return b }
