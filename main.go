package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	api "github.com/sacloud/api-client-go"
	"github.com/sacloud/api-client-go/profile"
)

func main() {
	profileName := flag.String("profile", "", "usacloud profile name")
	startAt := flag.String("start", "", "target maintenance start date in yyyy-mm-dd format")
	endAt := flag.String("end", "", "target maintenance end date in yyyy-mm-dd format")
	format := flag.String("format", "csv", "output format (csv, tsv, ltsv, or json)")
	indent := flag.Bool("indent", false, "enable indent for json output")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version())
		return
	}

	switch *format {
	case "csv", "tsv", "ltsv", "json":
	default:
		log.Fatal("format flag must be one of csv, tsv, ltsv, or json")
	}
	if err := run(*profileName, *startAt, *endAt, *format, *indent); err != nil {
		log.Fatal(err)
	}
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return info.Main.Version
}

const dateLayout = "2006-01-02"

func checkDateFormat(dateStr string) error {
	if dateStr == "" {
		return nil
	}
	_, err := time.Parse(dateLayout, dateStr)
	if err != nil {
		return errors.New("date format must be yyyy-mm-dd")
	}
	return nil
}

func run(profileName, startAt, endAt, outputFormat string, indent bool) error {
	opts, err := api.OptionsFromProfile(profileName)
	if err != nil {
		return err
	}
	prof := opts.ProfileConfigValue()
	// log.Printf("accessKey=%s, secret=%s", prof.AccessToken, prof.AccessTokenSecret)

	if err := checkDateFormat(startAt); err != nil {
		return err
	}
	if err := checkDateFormat(endAt); err != nil {
		return err
	}

	c := &http.Client{}
	maintenances, err := getMaintenances(c, prof, startAt, endAt)
	if err != nil {
		return err
	}
	// log.Printf("maintenances=%+v", maintenances)

	mainteInfoURLsInZones := maintenances.ToMainteInfoURLsInZones()
	zones := slices.Sorted(maps.Keys(mainteInfoURLsInZones))
	// log.Printf("mainteInfoURLsInZones=%+v", mainteInfoURLsInZones)
	mainteByInfoURLs := maintenances.ToMainteByInfoURLs()

	var mainteScheduledServers []MainteScheduledServer
	for _, zone := range zones {
		infoURLs := mainteInfoURLsInZones[zone]
		servers, err := getMainteScheduledServers(c, prof, zone, infoURLs)
		if err != nil {
			return err
		}
		// log.Printf("zone=%s, servers=%+v", zone, servers)
		for _, server := range servers.Servers {
			mainte := mainteByInfoURLs[server.Instance.Host.InfoURL]
			mainteScheduledServers = append(mainteScheduledServers, MainteScheduledServer{
				Zone:           zone,
				ID:             server.ID,
				Name:           server.Name,
				HostName:       server.HostName,
				HostServerName: server.Instance.Host.Name,
				MainteStartAt:  mainte.StartAt,
				MainteURL:      server.Instance.Host.InfoURL,
			})
		}
	}

	switch outputFormat {
	case "csv":
		if err := writeMainteScheduledServersInCSV(os.Stdout, ',', mainteScheduledServers); err != nil {
			return err
		}
	case "tsv":
		if err := writeMainteScheduledServersInCSV(os.Stdout, '\t', mainteScheduledServers); err != nil {
			return err
		}
	case "ltsv":
		if err := writeMainteScheduledServersInLTSV(os.Stdout, mainteScheduledServers); err != nil {
			return err
		}
	case "json":
		if err := writeMainteScheduledServersInJSON(os.Stdout, indent, mainteScheduledServers); err != nil {
			return err
		}
	}

	return nil
}

type MainteScheduledServer struct {
	Zone           string
	ID             string
	Name           string
	HostName       string
	HostServerName string
	MainteURL      string
	MainteStartAt  string
}

func writeMainteScheduledServersInCSV(w io.Writer, delim rune, servers []MainteScheduledServer) error {
	tw := csv.NewWriter(w)
	tw.Comma = delim
	if err := tw.Write([]string{"Zone", "ID", "Name", "HostName", "MainteStartAt", "HostServerName", "MainteURL"}); err != nil {
		return err
	}
	for _, s := range servers {
		if err := tw.Write([]string{s.Zone, s.ID, s.Name, s.HostName, s.MainteStartAt, s.HostServerName, s.MainteURL}); err != nil {
			return err
		}
	}
	tw.Flush()
	if err := tw.Error(); err != nil {
		return err
	}
	return nil
}

func writeMainteScheduledServersInLTSV(w io.Writer, servers []MainteScheduledServer) error {
	bw := bufio.NewWriter(w)
	for _, s := range servers {
		line := strings.Join([]string{
			"Zone:" + s.Zone,
			"ID:" + s.ID,
			"Name:" + s.Name,
			"HostName:" + s.HostName,
			"MainteStartAt:" + s.MainteStartAt,
			"HostServerName:" + s.HostServerName,
			"MainteURL:" + s.MainteURL,
		}, "\t") + "\n"
		if _, err := bw.WriteString(line); err != nil {
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return nil
}

func writeMainteScheduledServersInJSON(w io.Writer, indent bool, servers []MainteScheduledServer) error {
	enc := json.NewEncoder(w)
	if indent {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(servers); err != nil {
		return err
	}
	return nil
}

type MainteInfoURLsInZone struct {
	Zone     string
	InfoURLs []string
}

type MaintenanceseMeta struct {
	TotalPages int `json:"total_pages"`
	TotalCount int `json:"total_count"`
}

type MaintenanceseMaintenance struct {
	Zone    string `json:"zone"`
	StartAt string `json:"start_at"`
	InfoURL string `json:"info_url"`
}

type Maintenances struct {
	IsOK         bool                       `json:"is_ok"`
	Meta         MaintenanceseMeta          `json:"meta"`
	Maintenances []MaintenanceseMaintenance `json:"maintenances"`
}

func (m *Maintenances) ToMainteInfoURLsInZones() map[string][]string {
	infoURLsInZones := make(map[string][]string)
	for _, mainte := range m.Maintenances {
		infoURLs := infoURLsInZones[mainte.Zone]
		infoURLs = append(infoURLs, mainte.InfoURL)
		infoURLsInZones[mainte.Zone] = infoURLs
	}
	return infoURLsInZones
}

func (m *Maintenances) ToMainteByInfoURLs() map[string]*MaintenanceseMaintenance {
	mainteByInfoURLs := make(map[string]*MaintenanceseMaintenance)
	for i := range m.Maintenances {
		mainte := &m.Maintenances[i]
		mainteByInfoURLs[mainte.InfoURL] = mainte
	}
	return mainteByInfoURLs
}

const MaintenancesQueryFilterSearchTypeRelated = "related"

type MaintenancesQuery struct {
	Filter MaintenancesQueryFilter
}

type MaintenancesQueryFilter struct {
	StartAt    string `json:"start_at,omitempty"`
	EndAt      string `json:"end_at,omitempty"`
	SearchType string `json:"search_type,omitempty"`
	PageCount  int    `json:"page_count"`
}

type ServersQuery struct {
	Count   int
	Filter  ServersQueryFilter
	Include []string
	Sort    []string
}

type ServersQueryFilter struct {
	InstanceHostInfoURLs []string `json:"Instance.Host.InfoURL"`
}

type Servers struct {
	From    int
	Count   int
	Total   int
	Servers []ServersServer
	IsOK    bool   `json:"is_ok"`
	LogURL  string `json:"_log_url"`
}

type ServersServer struct {
	ID       string
	Name     string
	HostName string
	Instance ServersServerInstance
}

type ServersServerInstance struct {
	Host ServersServerInstanceHost
}

type ServersServerInstanceHost struct {
	Name    string
	InfoURL string
}

const serverQueryCount = 1000

func getMainteScheduledServers(c *http.Client, prof *profile.ConfigValue, zone string, mainteInfoURLs []string) (*Servers, error) {
	baseURL := fmt.Sprintf("https://secure.sakura.ad.jp/cloud/zone/%s/api/cloud/1.1/", zone)

	query := ServersQuery{
		Count: serverQueryCount,
		Filter: ServersQueryFilter{
			InstanceHostInfoURLs: mainteInfoURLs,
		},
		Include: []string{"Name", "HostName", "Instance.Server.ID", "Instance.Host.Name", "Instance.Host.InfoURL"},
		Sort:    []string{"Server.ID"},
	}
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	reqURL := baseURL + "server?" + url.QueryEscape(string(queryJSON))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(prof.AccessToken, prof.AccessTokenSecret)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// log.Printf("servers respBody=%s", string(respBodyBytes))

	s := &Servers{}
	if err := json.Unmarshal(respBodyBytes, s); err != nil {
		return nil, err
	}

	if s.Total > serverQueryCount {
		log.Printf("too many maintenance-scheduled servers returned: %d > %d", s.Total, serverQueryCount)
	}

	return s, nil
}

const maintenancesPageCount = 1000

func getMaintenances(c *http.Client, prof *profile.ConfigValue, startAt, endAt string) (*Maintenances, error) {
	query := MaintenancesQuery{
		Filter: MaintenancesQueryFilter{
			StartAt:    startAt,
			EndAt:      endAt,
			SearchType: MaintenancesQueryFilterSearchTypeRelated,
			PageCount:  maintenancesPageCount,
		},
	}
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	q := url.QueryEscape(string(queryJSON))
	req, err := http.NewRequest(http.MethodGet, "https://secure.sakura.ad.jp/cloud/api/global/1.0/maintenances?"+q, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(prof.AccessToken, prof.AccessTokenSecret)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// log.Printf("maintenances respBody=%s", string(respBodyBytes))

	m := &Maintenances{}
	if err := json.Unmarshal(respBodyBytes, m); err != nil {
		return nil, err
	}

	if m.Meta.TotalCount > maintenancesPageCount {
		return nil, errors.New("please make start date and end date more closer to each other (too many maintenances returned)")
	}

	return m, nil
}
