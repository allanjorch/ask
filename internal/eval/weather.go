package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (e *Engine) evalWeather(ctx context.Context, body string) []Result {
	place := strings.TrimSpace(body)
	if len(place) >= 3 && strings.EqualFold(place[:3], "in ") {
		place = strings.TrimSpace(place[3:])
	}
	place = strings.TrimSpace(place)
	if place == "" {
		return nil
	}
	g, err := e.geocode(ctx, place)
	if err != nil {
		return nil
	}
	w, err := e.forecast(ctx, g.Lat, g.Lon)
	if err != nil {
		return nil
	}
	title := fmt.Sprintf("%s  %s°C  %s", g.Label, formatNumber(w.temp), w.condition)
	sub := fmt.Sprintf("H %s° / L %s°", formatNumber(w.high), formatNumber(w.low))
	if w.wind != 0 {
		sub += fmt.Sprintf("  ·  %s km/h", formatNumber(w.wind))
	}
	return []Result{{
		Kind:     KindWeather,
		Title:    title,
		Subtitle: sub,
		Copy:     title,
		Rank:     75,
	}}
}

type geoResult struct {
	Lat, Lon float64
	Label    string
	Timezone string
}

type geoCache struct {
	res     geoResult
	fetched time.Time
}

func (e *Engine) geocode(ctx context.Context, place string) (geoResult, error) {
	key := strings.ToLower(strings.TrimSpace(place))
	e.mu.Lock()
	if e.geo == nil {
		e.geo = map[string]geoCache{}
	}
	if c, ok := e.geo[key]; ok && time.Since(c.fetched) < time.Hour {
		e.mu.Unlock()
		return c.res, nil
	}
	e.mu.Unlock()

	u := e.geoURL(place)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return geoResult{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := e.http().Do(req)
	if err != nil {
		return geoResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return geoResult{}, fmt.Errorf("geocode %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return geoResult{}, err
	}
	var parsed struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Admin1    string  `json:"admin1"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Timezone  string  `json:"timezone"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return geoResult{}, err
	}
	if len(parsed.Results) == 0 {
		return geoResult{}, fmt.Errorf("no place")
	}
	r := parsed.Results[0]
	label := r.Name
	if r.Country != "" {
		label += ", " + r.Country
	}
	out := geoResult{Lat: r.Latitude, Lon: r.Longitude, Label: label, Timezone: r.Timezone}
	e.mu.Lock()
	e.geo[key] = geoCache{res: out, fetched: time.Now()}
	e.mu.Unlock()
	return out, nil
}

func (e *Engine) geoURL(place string) string {
	if e.GeoURL != nil {
		return e.GeoURL(place)
	}
	return "https://geocoding-api.open-meteo.com/v1/search?count=1&name=" + url.QueryEscape(place)
}

type weather struct {
	temp, high, low, wind float64
	condition             string
}

func (e *Engine) forecast(ctx context.Context, lat, lon float64) (weather, error) {
	u := e.weatherURL(lat, lon)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return weather{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := e.http().Do(req)
	if err != nil {
		return weather{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return weather{}, fmt.Errorf("weather %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return weather{}, err
	}
	var parsed struct {
		Current struct {
			Temp        float64 `json:"temperature_2m"`
			Wind        float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			Max []float64 `json:"temperature_2m_max"`
			Min []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return weather{}, err
	}
	w := weather{
		temp:      parsed.Current.Temp,
		wind:      parsed.Current.Wind,
		condition: weatherCode(parsed.Current.WeatherCode),
	}
	if len(parsed.Daily.Max) > 0 {
		w.high = parsed.Daily.Max[0]
	}
	if len(parsed.Daily.Min) > 0 {
		w.low = parsed.Daily.Min[0]
	}
	return w, nil
}

func (e *Engine) weatherURL(lat, lon float64) string {
	if e.WeatherURL != nil {
		return e.WeatherURL(lat, lon)
	}
	return fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,weather_code,wind_speed_10m&daily=temperature_2m_max,temperature_2m_min&timezone=auto&forecast_days=1", lat, lon)
}

func weatherCode(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code == 1:
		return "Mostly clear"
	case code == 2:
		return "Partly cloudy"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Showers"
	case code >= 85 && code <= 86:
		return "Snow showers"
	case code == 95:
		return "Thunderstorm"
	case code == 96 || code == 99:
		return "Thunderstorm"
	default:
		return "—"
	}
}
