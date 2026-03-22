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

type Quote struct {
  Symbol string `json:"symbol"`
  Price float64 `json:"price"`
  ChangePct float64 `json:"changePct"`
  Score float64 `json:"score"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
}

type OptionIdea struct {
  Symbol string `json:"symbol"`
  Underlying string `json:"underlying"`
  Strategy string `json:"strategy"`
  Price float64 `json:"price"`
  Target float64 `json:"target"`
  Quantity int `json:"quantity"`
  ProfitLoss float64 `json:"profitLoss"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
}

type CryptoQuote struct {
  Product string `json:"product"`
  Price float64 `json:"price"`
  ChangePct float64 `json:"changePct"`
  Bias string `json:"bias"`
  Source string `json:"source"`
  UpdatedAt string `json:"updatedAt"`
}

type SpreadIdea struct {
  Underlying string `json:"underlying"`
  Direction string `json:"direction"`
  Idea string `json:"idea"`
  Source string `json:"source"`
}

type PanelFeed struct {
  GeneratedAt string `json:"generatedAt"`
  ProviderSummary string `json:"providerSummary"`
  Stocks []Quote `json:"stocks"`
  Options []OptionIdea `json:"options"`
  Crypto []CryptoQuote `json:"crypto"`
  Spreads []SpreadIdea `json:"spreads"`
  Alerts []map[string]string `json:"alerts"`
}

type yChartResp struct { Chart struct { Result []struct { Meta struct { RegularMarketPrice float64 `json:"regularMarketPrice"`; PreviousClose float64 `json:"previousClose"`; Currency string `json:"currency"` } `json:"meta"`; Indicators struct { Quote []struct { Close []*float64 `json:"close"` } `json:"quote"` } `json:"indicators"` } `json:"result"` } `json:"chart"` }

type coinbaseTicker struct { Price string `json:"price"` }

type yOptionResp struct { OptionChain struct { Result []struct { Quote struct { RegularMarketPrice float64 `json:"regularMarketPrice"`; RegularMarketPreviousClose float64 `json:"regularMarketPreviousClose"` } `json:"quote"`; Options []struct { Calls []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` } `json:"calls"`; Puts []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` } `json:"puts"` } `json:"options"` } `json:"result"` } `json:"optionChain"` }

func main() {
  feed := PanelFeed{GeneratedAt: time.Now().UTC().Format(time.RFC3339), ProviderSummary: "Yahoo public market data + Coinbase public prices"}
  symbols := []string{"NVDA","META","TSLA","AAPL","MSFT","AMZN","GOOGL","AMD","AVGO","NFLX","PLTR","SMCI","MU","COIN","HOOD","UBER","QQQ","SPY","MRVL","ORCL"}
  stocks := make([]Quote,0,len(symbols))
  for _, s := range symbols {
    if q, err := fetchYahooQuote(s); err == nil { stocks = append(stocks, q); time.Sleep(200*time.Millisecond) }
  }
  sort.Slice(stocks, func(i,j int) bool { return stocks[i].Score > stocks[j].Score })
  if len(stocks) > 20 { stocks = stocks[:20] }
  feed.Stocks = stocks

  options := []OptionIdea{}
  spreads := []SpreadIdea{}
  for _, s := range []string{"NVDA","META","TSLA","AAPL","MSFT"} {
    if idea, spread, err := fetchYahooOptionIdea(s); err == nil {
      options = append(options, idea)
      spreads = append(spreads, spread)
      time.Sleep(300*time.Millisecond)
    }
  }
  feed.Options = options
  feed.Spreads = spreads

  cryptos := []CryptoQuote{}
  for _, p := range []string{"BTC-USD","ETH-USD","SOL-USD","DOGE-USD"} {
    if c, err := fetchCoinbaseProduct(p); err == nil { cryptos = append(cryptos, c) }
  }
  feed.Crypto = cryptos

  feed.Alerts = []map[string]string{}
  if len(stocks) > 0 { feed.Alerts = append(feed.Alerts, map[string]string{"type":"stock","title":stocks[0].Symbol+" stock leader","message":"Top ranked stock from public market feed."}) }
  if len(options) > 0 { feed.Alerts = append(feed.Alerts, map[string]string{"type":"option","title":options[0].Underlying+" option idea","message":options[0].Strategy}) }
  if len(cryptos) > 0 { feed.Alerts = append(feed.Alerts, map[string]string{"type":"crypto","title":cryptos[0].Product+" crypto bias","message":cryptos[0].Bias}) }

  _ = os.MkdirAll("docs", 0755)
  b, _ := json.MarshalIndent(feed, "", "  ")
  _ = os.WriteFile("docs/public-feed.json", b, 0644)
  fmt.Println("generated public-feed.json")
}

func fetchYahooQuote(symbol string) (Quote, error) {
  url := "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?range=3mo&interval=1d"
  var resp yChartResp
  if err := getJSON(url, &resp); err != nil { return Quote{}, err }
  if len(resp.Chart.Result) == 0 { return Quote{}, fmt.Errorf("no result") }
  r := resp.Chart.Result[0]
  price := r.Meta.RegularMarketPrice
  prev := r.Meta.PreviousClose
  closes := r.Indicators.Quote[0].Close
  score := 50.0
  if len(closes) > 22 {
    last := pickLast(closes)
    m20 := pickBack(closes, 21)
    m60 := pickBack(closes, 60)
    r20 := pct(last, m20)
    r60 := pct(last, m60)
    score = clamp(50+r20*0.8+r60*0.5, 1, 99)
  }
  return Quote{Symbol:symbol, Price:round(price), ChangePct:round(pct(price, prev)), Score:round(score), Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, nil
}

func fetchYahooOptionIdea(symbol string) (OptionIdea, SpreadIdea, error) {
  url := "https://query2.finance.yahoo.com/v7/finance/options/" + symbol
  var resp yOptionResp
  if err := getJSON(url, &resp); err != nil { return OptionIdea{}, SpreadIdea{}, err }
  if len(resp.OptionChain.Result) == 0 || len(resp.OptionChain.Result[0].Options) == 0 { return OptionIdea{}, SpreadIdea{}, fmt.Errorf("no options") }
  quote := resp.OptionChain.Result[0].Quote
  opt := resp.OptionChain.Result[0].Options[0]
  price := quote.RegularMarketPrice
  prev := quote.RegularMarketPreviousClose
  direction := "bullish"
  if price < prev { direction = "bearish" }
  if direction == "bullish" && len(opt.Calls) > 0 {
    c := bestCall(opt.Calls)
    target := round(c.LastPrice * 1.25)
    qty := 20
    return OptionIdea{Symbol:c.ContractSymbol, Underlying:symbol, Strategy:fmt.Sprintf("Bull call debit spread around %.0f strike", c.Strike), Price:round(c.LastPrice), Target:target, Quantity:qty, ProfitLoss:round((target-c.LastPrice)*float64(qty)), Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, SpreadIdea{Underlying:symbol, Direction:"bullish", Idea:fmt.Sprintf("Buy near %.0fC and sell higher strike call", c.Strike), Source:"yahoo_public"}, nil
  }
  if len(opt.Puts) > 0 {
    p := bestPut(opt.Puts)
    target := round(p.LastPrice * 1.22)
    qty := 20
    return OptionIdea{Symbol:p.ContractSymbol, Underlying:symbol, Strategy:fmt.Sprintf("Bear put debit spread around %.0f strike", p.Strike), Price:round(p.LastPrice), Target:target, Quantity:qty, ProfitLoss:round((target-p.LastPrice)*float64(qty)), Source:"yahoo_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, SpreadIdea{Underlying:symbol, Direction:"bearish", Idea:fmt.Sprintf("Buy near %.0fP and sell lower strike put", p.Strike), Source:"yahoo_public"}, nil
  }
  return OptionIdea{}, SpreadIdea{}, fmt.Errorf("no suitable chain")
}

func fetchCoinbaseProduct(product string) (CryptoQuote, error) {
  url := "https://api.exchange.coinbase.com/products/" + product + "/ticker"
  var t coinbaseTicker
  if err := getJSON(url, &t); err != nil { return CryptoQuote{}, err }
  price := atof(t.Price)
  bias := "neutral"
  if price > 0 { bias = "bullish" }
  return CryptoQuote{Product:product, Price:round(price), ChangePct:0, Bias:bias, Source:"coinbase_public", UpdatedAt:time.Now().UTC().Format(time.RFC3339)}, nil
}

func bestCall(calls []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` }) struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` } {
  sort.Slice(calls, func(i,j int) bool { return calls[i].OpenInterest + calls[i].Volume > calls[j].OpenInterest + calls[j].Volume })
  return calls[0]
}
func bestPut(puts []struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` }) struct { ContractSymbol string `json:"contractSymbol"`; Strike float64 `json:"strike"`; LastPrice float64 `json:"lastPrice"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Volume int `json:"volume"`; OpenInterest int `json:"openInterest"`; ImpliedVolatility float64 `json:"impliedVolatility"` } {
  sort.Slice(puts, func(i,j int) bool { return puts[i].OpenInterest + puts[i].Volume > puts[j].OpenInterest + puts[j].Volume })
  return puts[0]
}

func getJSON(url string, v any) error {
  client := &http.Client{Timeout: 20*time.Second}
  req, _ := http.NewRequest(http.MethodGet, url, nil)
  req.Header.Set("User-Agent", "Mozilla/5.0")
  res, err := client.Do(req)
  if err != nil { return err }
  defer res.Body.Close()
  if res.StatusCode >= 400 { b,_ := io.ReadAll(res.Body); return fmt.Errorf("http %d: %s", res.StatusCode, string(b)) }
  b, _ := io.ReadAll(res.Body)
  return json.Unmarshal(b, v)
}
func pickLast(xs []*float64) float64 { for i:=len(xs)-1;i>=0;i-- { if xs[i]!=nil { return *xs[i] } }; return 0 }
func pickBack(xs []*float64, back int) float64 { idx:=len(xs)-1-back; if idx<0 { idx=0 }; for i:=idx;i>=0;i-- { if xs[i]!=nil { return *xs[i] } }; return pickLast(xs) }
func pct(curr, prev float64) float64 { if prev==0 { return 0 }; return (curr-prev)/prev*100 }
func clamp(v, lo, hi float64) float64 { if v<lo { return lo }; if v>hi { return hi }; return v }
func round(v float64) float64 { return math.Round(v*100)/100 }
func atof(s string) float64 { var v float64; fmt.Sscanf(s, "%f", &v); return v }
