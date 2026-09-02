// Package churchlocator is an installable plugin that finds nearby churches
// using reliable open-source GPS/geodata services from the OpenStreetMap
// ecosystem (no API key required):
//
//   - Nominatim  — geocodes a place name / address / city to lat/lng.
//   - Overpass   — queries OSM for places of worship around a coordinate.
//
// Results are cached in the store so repeat lookups work offline.
package churchlocator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gigatone/biblelearn/internal/plugins"
	"github.com/gigatone/biblelearn/internal/store"
)

const (
	// PluginName is the registry identifier for this plugin.
	PluginName = "church-locator"
	// OverpassEndpoint is the public open-source Overpass API.
	OverpassEndpoint = "https://overpass-api.de/api/interpreter"
	// NominatimEndpoint is the public open-source geocoding service.
	NominatimEndpoint = "https://nominatim.openstreetmap.org/search"
)

// Church is one place-of-worship result.
type Church struct {
	Name       string
	Denom      string // denomination or religion tag, when present
	Address    string
	Lat        float64
	Lng        float64
	DistanceKm float64 // approximate straight-line distance from the search point
}

// Plugin is the installable capability definition.
func Plugin() *plugins.Plugin {
	return &plugins.Plugin{
		Name:            PluginName,
		DisplayName:     "Church Locator",
		Description:     "Find nearby churches via reliable open-source OpenStreetMap geodata (Overpass + Nominatim), with offline caching.",
		Version:         "0.1.0",
		RequiresNetwork: true,
	}
}

// Geocode resolves a place query (city, address, landmark) to coordinates via
// the open-source Nominatim service. Returns the best match.
func Geocode(ctx context.Context, s *store.Store, query string) (lat, lng float64, name string, err error) {
	if cached, ok, _ := s.GetCache(ctx, "geo:"+query); ok {
		var c struct {
			Lat  float64 `json:"lat"`
			Lng  float64 `json:"lng"`
			Name string  `json:"name"`
		}
		if json.Unmarshal([]byte(cached), &c) == nil {
			return c.Lat, c.Lng, c.Name, nil
		}
	}
	u := NominatimEndpoint + "?q=" + url.QueryEscape(query) +
		"&format=json&limit=1&addressdetails=1"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return 0, 0, "", err
	}
	req.Header.Set("User-Agent", "ExaltedTerminal/0.1 (bible study tool)")
	req.Header.Set("Accept-Language", "en")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("geocode status %d", resp.StatusCode)
	}
	var results []struct {
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, "", err
	}
	if len(results) == 0 || results[0].Lat == "" {
		return 0, 0, "", fmt.Errorf("no location found for %q", query)
	}
	lat, _ = strconv.ParseFloat(results[0].Lat, 64)
	lng, _ = strconv.ParseFloat(results[0].Lon, 64)
	name = results[0].DisplayName
	enc, _ := json.Marshal(map[string]any{"lat": lat, "lng": lng, "name": name})
	_ = s.PutCache(ctx, "geo:"+query, string(enc), 30*24*time.Hour)
	return lat, lng, name, nil
}

// LocateNear returns places of worship within radiusKm of a coordinate via the
// open-source Overpass API, sorted by distance (nearest first).
func LocateNear(ctx context.Context, s *store.Store, lat, lng, radiusKm float64) ([]Church, error) {
	if radiusKm <= 0 {
		radiusKm = 5
	}
	key := fmt.Sprintf("near:%v,%v,%v", lat, lng, radiusKm)
	if cached, ok, _ := s.GetCache(ctx, key); ok {
		var out []Church
		if json.Unmarshal([]byte(cached), &out) == nil && len(out) > 0 {
			return out, nil
		}
	}
	q := fmt.Sprintf(`[out:json][timeout:25];
(nwr["amenity"="place_of_worship"](around:%d,%f,%f););
out center tags 30;`, int(radiusKm*1000), lat, lng)
	body, err := overpassPOST(ctx, q)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Elements []struct {
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Tags struct {
				Name         string `json:"name"`
				Denomination string `json:"denomination"`
				Religion     string `json:"religion"`
				Street       string `json:"addr:street"`
				City         string `json:"addr:city"`
				Housenumber  string `json:"addr:housenumber"`
			} `json:"tags"`
			Center struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []Church
	for _, e := range parsed.Elements {
		clat, clng := e.Lat, e.Lon
		if clat == 0 && clng == 0 {
			clat, clng = e.Center.Lat, e.Center.Lon
		}
		if clat == 0 && clng == 0 {
			continue
		}
		c := Church{
			Name:       e.Tags.Name,
			Denom:      denomLabel(e.Tags.Denomination, e.Tags.Religion),
			Address:    buildAddress(e.Tags.Housenumber, e.Tags.Street, e.Tags.City),
			Lat:        clat,
			Lng:        clng,
			DistanceKm: haversineKM(lat, lng, clat, clng),
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no churches found within %g km", radiusKm)
	}
	// nearest first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].DistanceKm < out[j-1].DistanceKm; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	enc, _ := json.Marshal(out)
	_ = s.PutCache(ctx, key, string(enc), 30*24*time.Hour)
	return out, nil
}

// Locate resolves a place query then returns churches near it.
func Locate(ctx context.Context, s *store.Store, query string, radiusKm float64) (string, []Church, error) {
	lat, lng, name, err := Geocode(ctx, s, query)
	if err != nil {
		return "", nil, err
	}
	churches, err := LocateNear(ctx, s, lat, lng, radiusKm)
	return name, churches, err
}

func overpassPOST(ctx context.Context, query string) ([]byte, error) {
	form := url.Values{}
	form.Set("data", query)
	req, err := http.NewRequestWithContext(ctx, "POST", OverpassEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "ExaltedTerminal/0.1 (bible study tool)")
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
}

func denomLabel(denom, religion string) string {
	switch {
	case denom != "":
		return denom
	case religion != "":
		return religion
	default:
		return "Christian"
	}
}

func buildAddress(hn, street, city string) string {
	parts := []string{}
	if hn != "" || street != "" {
		parts = append(parts, strings.TrimSpace(hn+" "+street))
	}
	if city != "" {
		parts = append(parts, city)
	}
	return strings.Join(parts, ", ")
}

// haversineKM returns the great-circle distance in kilometres between two
// coordinates.
func haversineKM(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180.0 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
