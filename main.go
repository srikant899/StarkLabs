package main

import (
    "encoding/json"
    "fmt"
    "math"
    "math/rand"
    "net/http"
    "os"
    "sort"
    "strings"
    "time"
)

type AssetType string

const (
    Stock  AssetType = "stock"
    Option AssetType = "option"
    Crypto AssetType = "crypto"
)

type MarketSnapshot struct {
    Symbol        string    `json:"symbol"`
    AssetType     AssetType `json:"assetType"`
    Price         float64   `json:"price"`
    PrevClose     float64   `json:"prevClose"`
    Volume        float64   `json:"volume"`
    AvgVolume     float64   `json:"avgVolume"`
    OpenInterest  float64   `json:"openInterest,omitempty"`
    Bid           float64   `json:"bid,omitempty"`
    Ask           float64   `json:"ask,omitempty"`
    ImpliedVol    float64   `json:"impliedVol,omitempty"`
    Delta         float64   `json:"delta,omitempty"`
    Gamma         float64   `json:"gamma,omitempty"`
    Theta         float64   `json:"theta,omitempty"`
    Vega          float64   `json:"vega,omitempty"`
    Underlying    string    `json:"underlying,omitempty"`
    Expiry        string    `json:"expiry,omitempty"`
    Strike        float64   `json:"strike,omitempty"`
    Side          string    `json:"side,omitempty"`
    Timestamp     time.Time `json:"timestamp"`
}

type Signal struct {
    Symbol           string    `json:"symbol"`
    AssetType        AssetType `json:"assetType"`
    Score            float64   `json:"score"`
    Direction        string    `json:"direction"`
    Confidence       float64   `json:"confidence"`
    ExpectedMovePct  float64   `json:"expectedMovePct"`
    Price            float64   `json:"price"`
    Reason           string    `json:"reason"`
    SpreadIdea       string    `json:"spreadIdea,omitempty"`
    GeneratedAt      time.Time `json:"generatedAt"`
}

type SignalFile struct {
    GeneratedAt time.Time `json:"generatedAt"`
    Provider    string    `json:"provider"`
    Signals     []Signal  `json:"signals"`
}

func main() {
    provider := getenv("DATA_PROVIDER", "mock")

    snapshots, err := loadSnapshots(provider)
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to load market data: %v\n", err)
        os.Exit(1)
    }

    signals := generateSignals(snapshots)
    payload := SignalFile{
        GeneratedAt: time.Now().UTC(),
        Provider:    provider,
        Signals:     signals,
    }

    if err := os.MkdirAll("docs", 0o755); err != nil {
        panic(err)
    }

    f, err := os.Create("docs/signals.json")
    if err != nil {
        panic(err)
    }
    defer f.Close()

    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(payload); err != nil {
        panic(err)
    }

    fmt.Printf("generated %d signals using provider=%s\n", len(signals), provider)
}

func loadSnapshots(provider string) ([]MarketSnapshot, error) {
    switch strings.ToLower(provider) {
    case "mock":
        return mockSnapshots(), nil
    case "alpaca":
        return alpacaLikeSnapshots(), nil
    default:
        return mockSnapshots(), nil
    }
}

func mockSnapshots() []MarketSnapshot {
    now := time.Now().UTC()
    return []MarketSnapshot{
        {Symbol: "NVDA", AssetType: Stock, Price: 142.4, PrevClose: 138.2, Volume: 89000000, AvgVolume: 51000000, Timestamp: now},
        {Symbol: "META", AssetType: Stock, Price: 611.0, PrevClose: 603.4, Volume: 23200000, AvgVolume: 15400000, Timestamp: now},
        {Symbol: "TSLA", AssetType: Stock, Price: 188.5, PrevClose: 194.2, Volume: 117000000, AvgVolume: 92000000, Timestamp: now},
        {Symbol: "BTC-USD", AssetType: Crypto, Price: 87320, PrevClose: 86110, Volume: 3.2e10, AvgVolume: 2.2e10, Timestamp: now},
        {Symbol: "ETH-USD", AssetType: Crypto, Price: 4680, PrevClose: 4525, Volume: 1.5e10, AvgVolume: 1.1e10, Timestamp: now},
        {Symbol: "NVDA-20260417-150-C", AssetType: Option, Price: 8.6, PrevClose: 6.8, Volume: 58200, AvgVolume: 22000, OpenInterest: 73000, Bid: 8.5, Ask: 8.7, ImpliedVol: 0.46, Delta: 0.42, Gamma: 0.03, Theta: -0.07, Vega: 0.12, Underlying: "NVDA", Expiry: "2026-04-17", Strike: 150, Side: "call", Timestamp: now},
        {Symbol: "META-20260417-640-C", AssetType: Option, Price: 7.2, PrevClose: 5.9, Volume: 14400, AvgVolume: 5200, OpenInterest: 24100, Bid: 7.1, Ask: 7.3, ImpliedVol: 0.31, Delta: 0.37, Gamma: 0.02, Theta: -0.05, Vega: 0.1, Underlying: "META", Expiry: "2026-04-17", Strike: 640, Side: "call", Timestamp: now},
        {Symbol: "TSLA-20260417-175-P", AssetType: Option, Price: 9.8, PrevClose: 8.2, Volume: 44100, AvgVolume: 17000, OpenInterest: 55100, Bid: 9.7, Ask: 9.9, ImpliedVol: 0.59, Delta: -0.46, Gamma: 0.028, Theta: -0.09, Vega: 0.16, Underlying: "TSLA", Expiry: "2026-04-17", Strike: 175, Side: "put", Timestamp: now},
    }
}

func alpacaLikeSnapshots() []MarketSnapshot {
    // Placeholder live-mode hook. In production, replace with real API reads.
    // This keeps the app functional until secrets and provider integration are added.
    return mockSnapshots()
}

func generateSignals(snaps []MarketSnapshot) []Signal {
    out := make([]Signal, 0, len(snaps))
    for _, s := range snaps {
        score, direction, conf, move, reason := mfveScore(s)
        signal := Signal{
            Symbol:          s.Symbol,
            AssetType:       s.AssetType,
            Score:           round(score),
            Direction:       direction,
            Confidence:      round(conf),
            ExpectedMovePct: round(move),
            Price:           s.Price,
            Reason:          reason,
            GeneratedAt:     time.Now().UTC(),
        }
        if s.AssetType == Option {
            signal.SpreadIdea = buildSpreadIdea(s, direction)
        }
        out = append(out, signal)
    }

    sort.Slice(out, func(i, j int) bool {
        return out[i].Score > out[j].Score
    })

    if len(out) > 12 {
        out = out[:12]
    }
    return out
}

// MFVE = Momentum + Flow + Volatility Efficiency
func mfveScore(s MarketSnapshot) (float64, string, float64, float64, string) {
    pctChange := pct(s.Price, s.PrevClose)
    volRatio := safeDiv(s.Volume, max(s.AvgVolume, 1))
    spreadCost := safeDiv(s.Ask-s.Bid, max(s.Price, 0.01))

    momentum := clamp(pctChange*8, -25, 25)
    flow := clamp((volRatio-1.0)*18, -10, 35)
    efficiency := clamp((1.0-spreadCost)*15, 0, 15)

    optionsEdge := 0.0
    if s.AssetType == Option {
        oiBoost := clamp(math.Log1p(max(s.OpenInterest, 1))/2.0, 0, 8)
        greekPulse := clamp(math.Abs(s.Delta)*8+math.Abs(s.Gamma)*60+math.Abs(s.Vega)*10-math.Abs(s.Theta)*15, 0, 18)
        ivPenalty := clamp((s.ImpliedVol-0.25)*10, 0, 8)
        optionsEdge = oiBoost + greekPulse - ivPenalty
    }

    cryptoEdge := 0.0
    if s.AssetType == Crypto {
        cryptoEdge = clamp(math.Log1p(volRatio)*8, 0, 10)
    }

    jitter := (rand.Float64() - 0.5) * 1.2
    total := 50 + momentum + flow + efficiency + optionsEdge + cryptoEdge + jitter
    total = clamp(total, 1, 99)

    direction := "bullish"
    if pctChange < 0 {
        direction = "bearish"
    }

    confidence := clamp(40+math.Abs(momentum)*0.9+flow*0.7+efficiency*0.4+optionsEdge*0.4+cryptoEdge*0.5, 1, 99)
    expectedMove := clamp(math.Abs(pctChange)*1.6+math.Log1p(volRatio)*2.5, 0.2, 18)

    reasonParts := []string{
        fmt.Sprintf("price change %.2f%%", pctChange),
        fmt.Sprintf("volume ratio %.2fx", volRatio),
        fmt.Sprintf("execution spread %.2f%%", spreadCost*100),
    }
    if s.AssetType == Option {
        reasonParts = append(reasonParts,
            fmt.Sprintf("OI %.0f", s.OpenInterest),
            fmt.Sprintf("IV %.2f", s.ImpliedVol),
            fmt.Sprintf("delta %.2f", s.Delta),
        )
    }
    return total, direction, confidence, expectedMove, strings.Join(reasonParts, "; ")
}

func buildSpreadIdea(s MarketSnapshot, direction string) string {
    width := 5.0
    lower := s.Strike
    upper := s.Strike + width
    if strings.EqualFold(s.Side, "put") || direction == "bearish" {
        upper = s.Strike
        lower = s.Strike - width
        return fmt.Sprintf("Bear put debit spread: buy %.0fP / sell %.0fP exp %s", upper, lower, s.Expiry)
    }
    return fmt.Sprintf("Bull call debit spread: buy %.0fC / sell %.0fC exp %s", lower, upper, s.Expiry)
}

func pct(curr, prev float64) float64 { return safeDiv(curr-prev, max(prev, 0.0001)) * 100 }
func round(v float64) float64 { return math.Round(v*100) / 100 }
func clamp(v, lo, hi float64) float64 { if v < lo { return lo }; if v > hi { return hi }; return v }
func safeDiv(a, b float64) float64 { if b == 0 { return 0 }; return a / b }
func max(a, b float64) float64 { if a > b { return a }; return b }
func getenv(k, d string) string { v := os.Getenv(k); if v == "" { return d }; return v }

// Optional small health handler for local runs.
func init() {
    if os.Getenv("RUN_HTTP") == "1" {
        go func() {
            http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
            _ = http.ListenAndServe(":8080", nil)
        }()
    }
}
