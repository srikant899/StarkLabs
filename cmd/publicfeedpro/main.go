package main

import (
  "encoding/json"
  "fmt"
  "io"
  "math"
  "net/http"
  "os"
  "sort"
  "time"
)

type TargetRow struct {
  Horizon string `json:"horizon"`
  TargetPrice float64 `json:"targetPrice"`
  Quantity int `json:"quantity"`
  ProfitLoss float64 `json:"profitLoss"`
  StopLoss float64 `json:"stopLoss"`
  Confidence float64 `json:"confidence"`
  UpsidePct float64 `json:"upsidePct"`
}

type StockCard struct {
  Symbol string `json:"symbol"`
  Price float64 `json:"price"`
  ChangePct float64 `json:"changePct"`
  Score float64 `json:"score"`
  Risk string `json:"risk"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
  Thesis string `json:"thesis"`
  Targets []TargetRow `json:"targets"`
  Spark []float64 `json:"spark"`
}

type OptionCard struct {
  Symbol string `json:"symbol"`
  Underlying string `json:"underlying"`
  Strategy string `json:"strategy"`
  Price float64 `json:"price"`
  Target float64 `json:"target"`
  Quantity int `json:"quantity"`
  ProfitLoss float64 `json:"profitLoss"`
  StopLoss float64 `json:"stopLoss"`
  Confidence float64 `json:"confidence"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
}

type CryptoCard struct {
  Product string `json:"product"`
  Price float64 `json:"price"`
  Bias string `json:"bias"`
  StopLoss float64 `json:"stopLoss"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
}

type SpreadCard struct {
  Underlying string `json:"underlying"`
  Direction string `json:"direction"`
  Idea string `json:"idea"`
  Debit float64 `json:"debit"`
  MaxProfit float64 `json:"maxProfit"`
  StopLoss float64 `json:"stopLoss"`
  Confidence float64 `json:"confidence"`
  Source string `json:"source"`
}

type Feed struct {
  GeneratedAt string `json:"generatedAt"`
  ProviderSummary string `json:"providerSummary"`
  Stocks []StockCard `json:"stocks"`
  Options []OptionCard `json:"options"`
  Crypto []CryptoCard `json:"crypto"`
  Spreads []SpreadCard `json:"spreads"`
}

type yChartResp struct { Chart struct { Result []struct { Meta struct { RegularMarketPrice float64 `json:"regularMarketPrice"`; PreviousClose float64 `json:"previousClose"` } `json:"meta"`; Indicators struct { Quote []struct { Close []*float64 `json:"close"` } `json:"quote"` } `json:"indicators"` } `json:"result"` } `json:"chart"` }

type coinbaseTicker struct { Price string `json:"price"` }

type yOptionResp struct { OptionChain struct { Result []struct { Options []struct { Calls []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"` } `json:"calls"`; Puts []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"` } `json:"puts"` } `json:"options"` } `json:"result"` } `json:"optionChain"` }

type optionLite struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"` }

func main() {
  symbols := []string{"NVDA","META","TSLA","AAPL","MSFT","AMZN","GOOGL","AMD","AVGO","NFLX","PLTR","SMCI","MU","COIN","HOOD","UBER","QQQ","SPY","MRVL","ORCL"}
  feed := Feed{GeneratedAt: time.Now().UTC().Format(time.RFC3339), ProviderSummary: "Yahoo public market data + Coinbase public prices"}
  for _, s := range symbols {
    if card, err := fetchStockCard(s); err == nil { feed.Stocks = append(feed.Stocks, card); time.Sleep(180*time.Millisecond) }
  }
  sort.Slice(feed.Stocks, func(i,j int) bool { return feed.Stocks[i].Score > feed.Stocks[j].Score })
  if len(feed.Stocks) > 20 { feed.Stocks = feed.Stocks[:20] }

  for _, s := range []string{"NVDA","META","TSLA","AAPL","MSFT"} {
    card, spread, err := fetchOptionAndSpread(s)
    if err == nil {
      feed.Options = append(feed.Options, card)
      feed.Spreads = append(feed.Spreads, spread)
    } else {
      if fallback, sp := modeledFallback(s, feed.Stocks); fallback.Symbol != "" {
        feed.Options = append(feed.Options, fallback)
        feed.Spreads = append(feed.Spreads, sp)
      }
    }
    time.Sleep(220*time.Millisecond)
  }

  for _, p := range []string{"BTC-USD","ETH-USD","SOL-USD","DOGE-USD"} {
    if c, err := fetchCrypto(p); err == nil { feed.Crypto = append(feed.Crypto, c) }
  }

  _ = os.MkdirAll("docs", 0755)
  b, _ := json.MarshalIndent(feed, "", "  ")
  _ = os.WriteFile("docs/public-feed-pro.json", b, 0644)
  fmt.Println("generated public-feed-pro.json")
}

func fetchStockCard(symbol string) (StockCard, error) {
  var resp yChartResp
  if err := getJSON("https://query1.finance.yahoo.com/v8/finance/chart/"+symbol+"?range=6mo&interval=1d", &resp); err != nil { return StockCard{}, err }
  if len(resp.Chart.Result) == 0 || len(resp.Chart.Result[0].Indicators.Quote) == 0 { return StockCard{}, fmt.Errorf("no result") }
  r := resp.Chart.Result[0]
  closes := r.Indicators.Quote[0].Close
  price := r.Meta.RegularMarketPrice
  prev := r.Meta.PreviousClose
  spark := compactSpark(closes, 28)
  v20 := realizedVol(closes, 20)
  r20 := pct(price, pickBack(closes, 21))
  r60 := pct(price, pickBack(closes, 60))
  score := clamp(50+r20*0.8+r60*0.45-v20*25, 1, 99)
  risk := "medium"; if v20 < 0.25 { risk = "low" } else if v20 > 0.5 { risk = "high" }
  targets := buildTargets(price, v20, r20, r60)
  thesis := fmt.Sprintf("20d %.1f%%, 60d %.1f%%, realized vol %.1f%%", r20, r60, v20*100)
  return StockCard{Symbol:symbol, Price:round(price), ChangePct:round(pct(price, prev)), Score:round(score), Risk:risk, Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339), Thesis:thesis, Targets:targets, Spark:spark}, nil
}

func fetchOptionAndSpread(symbol string) (OptionCard, SpreadCard, error) {
  var resp yOptionResp
  if err := getJSON("https://query2.finance.yahoo.com/v7/finance/options/"+symbol, &resp); err != nil { return OptionCard{}, SpreadCard{}, err }
  if len(resp.OptionChain.Result) == 0 || len(resp.OptionChain.Result[0].Options) == 0 { return OptionCard{}, SpreadCard{}, fmt.Errorf("no chain") }
  opt := resp.OptionChain.Result[0].Options[0]
  if len(opt.Calls) > 0 {
    c := bestCallLite(opt.Calls)
    tgt := round(c.LastPrice*1.22)
    stop := round(c.LastPrice*0.78)
    pl := round((tgt-c.LastPrice)*20)
    return OptionCard{Symbol:c.ContractSymbol, Underlying:symbol, Strategy:fmt.Sprintf("Bull call setup near %.0f strike", c.Strike), Price:round(c.LastPrice), Target:tgt, Quantity:20, ProfitLoss:pl, StopLoss:stop, Confidence:73, Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, SpreadCard{Underlying:symbol, Direction:"bullish", Idea:fmt.Sprintf("Buy %.0fC and sell next higher call", c.Strike), Debit:round(c.LastPrice), MaxProfit:round(c.LastPrice*0.8*100), StopLoss:stop, Confidence:70, Source:"modeled_from_public_chain"}, nil
  }
  if len(opt.Puts) > 0 {
    p := bestPutLite(opt.Puts)
    tgt := round(p.LastPrice*1.20)
    stop := round(p.LastPrice*0.80)
    pl := round((tgt-p.LastPrice)*20)
    return OptionCard{Symbol:p.ContractSymbol, Underlying:symbol, Strategy:fmt.Sprintf("Bear put setup near %.0f strike", p.Strike), Price:round(p.LastPrice), Target:tgt, Quantity:20, ProfitLoss:pl, StopLoss:stop, Confidence:71, Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, SpreadCard{Underlying:symbol, Direction:"bearish", Idea:fmt.Sprintf("Buy %.0fP and sell next lower put", p.Strike), Debit:round(p.LastPrice), MaxProfit:round(p.LastPrice*0.8*100), StopLoss:stop, Confidence:69, Source:"modeled_from_public_chain"}, nil
  }
  return OptionCard{}, SpreadCard{}, fmt.Errorf("no contracts")
}

func modeledFallback(symbol string, stocks []StockCard) (OptionCard, SpreadCard) {
  for _, s := range stocks {
    if s.Symbol == symbol {
      bull := s.ChangePct >= 0
      price := round(math.Max(s.Price*0.03, 1.25))
      target := round(price*1.18)
      stop := round(price*0.82)
      strat := "Bull call debit spread"
      idea := "Buy near-the-money call and sell higher strike call"
      dir := "bullish"
      if !bull {
        strat = "Bear put debit spread"
        idea = "Buy near-the-money put and sell lower strike put"
        dir = "bearish"
      }
      return OptionCard{Symbol:symbol+"-MODELED", Underlying:symbol, Strategy:strat, Price:price, Target:target, Quantity:20, ProfitLoss:round((target-price)*20), StopLoss:stop, Confidence:64, Source:"modeled_from_stock_price", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, SpreadCard{Underlying:symbol, Direction:dir, Idea:idea, Debit:price, MaxProfit:round((target-price)*100), StopLoss:stop, Confidence:62, Source:"modeled_from_stock_price"}
    }
  }
  return OptionCard{}, SpreadCard{}
}

func fetchCrypto(product string) (CryptoCard, error) {
  var t coinbaseTicker
  if err := getJSON("https://api.exchange.coinbase.com/products/"+product+"/ticker", &t); err != nil { return CryptoCard{}, err }
  px := atof(t.Price)
  return CryptoCard{Product:product, Price:round(px), Bias:"bullish", StopLoss:round(px*0.93), Source:"coinbase_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, nil
}

func buildTargets(price, vol, r20, r60 float64) []TargetRow {
  drift := clamp((r20/100)*2.1+(r60/100)*1.4, -0.25, 0.45)
  horizons := []struct{ label string; t float64 }{{"1 week",5.0/252.0},{"1 month",21.0/252.0},{"3 months",63.0/252.0},{"6 months",126.0/252.0},{"12 months",1.0}}
  out := make([]TargetRow,0,len(horizons))
  for _, h := range horizons {
    expected := price * math.Exp((drift-0.5*vol*vol)*h.t)
    band := price * vol * math.Sqrt(h.t)
    target := round(expected + band*0.35)
    stop := round(math.Max(price-band*0.85, price*0.72))
    conf := round(clamp(76-vol*38+h.t*10, 38, 90))
    out = append(out, TargetRow{Horizon:h.label, TargetPrice:target, Quantity:20, ProfitLoss:round((target-price)*20), StopLoss:stop, Confidence:conf, UpsidePct:round((target/price-1)*100)})
  }
  return out
}

func compactSpark(xs []*float64, n int) []float64 {
  vals := []float64{}
  for _, x := range xs { if x != nil { vals = append(vals, round(*x)) } }
  if len(vals) <= n { return vals }
  step := float64(len(vals)-1) / float64(n-1)
  out := make([]float64,0,n)
  for i:=0;i<n;i++ { idx := int(math.Round(float64(i)*step)); if idx >= len(vals) { idx = len(vals)-1 }; out = append(out, vals[idx]) }
  return out
}

func realizedVol(xs []*float64, n int) float64 {
  vals := []float64{}
  for _, x := range xs { if x != nil { vals = append(vals, *x) } }
  if len(vals) < n+1 { return 0.35 }
  vals = vals[len(vals)-n-1:]
  rs := make([]float64,0,n)
  for i:=1;i<len(vals);i++ { rs = append(rs, math.Log(vals[i]/vals[i-1])) }
  m := 0.0; for _, r := range rs { m += r }; m /= float64(len(rs))
  v := 0.0; for _, r := range rs { d := r-m; v += d*d }; v /= math.Max(1,float64(len(rs)-1))
  return math.Sqrt(v) * math.Sqrt(252)
}

func bestCallLite(calls []optionLite) optionLite { sort.Slice(calls, func(i,j int) bool { return calls[i].OpenInterest + calls[i].Volume > calls[j].OpenInterest + calls[j].Volume }); return calls[0] }
func bestPutLite(puts []optionLite) optionLite { sort.Slice(puts, func(i,j int) bool { return puts[i].OpenInterest + puts[i].Volume > puts[j].OpenInterest + puts[j].Volume }); return puts[0] }
func getJSON(url string, v any) error { client := &http.Client{Timeout:20*time.Second}; req,_ := http.NewRequest(http.MethodGet, url, nil); req.Header.Set("User-Agent","Mozilla/5.0"); res, err := client.Do(req); if err != nil { return err }; defer res.Body.Close(); if res.StatusCode >= 400 { b,_ := io.ReadAll(res.Body); return fmt.Errorf("http %d: %s", res.StatusCode, string(b)) }; b,_ := io.ReadAll(res.Body); return json.Unmarshal(b,v) }
func pickBack(xs []*float64, back int) float64 { idx:=len(xs)-1-back; if idx<0 { idx=0 }; for i:=idx;i>=0;i-- { if xs[i]!=nil { return *xs[i] } }; for i:=len(xs)-1;i>=0;i-- { if xs[i]!=nil { return *xs[i] } }; return 0 }
func pct(curr, prev float64) float64 { if prev == 0 { return 0 }; return (curr-prev)/prev*100 }
func clamp(v, lo, hi float64) float64 { if v<lo { return lo }; if v>hi { return hi }; return v }
func round(v float64) float64 { return math.Round(v*100)/100 }
func atof(s string) float64 { var v float64; fmt.Sscanf(s, "%f", &v); return v }
