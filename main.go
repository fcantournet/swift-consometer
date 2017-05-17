package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"
	"sync"

	"net/http"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/marpaia/graphite-golang"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

// This is the share of time dedicated to each stage of the pipeline.
// We use this to calculate the timeout for each stage based on global timeout.
const (
	tsSwift    = 10
	tsRabbitMQ = 3
	tsReport   = 1
	tsSum      = tsSwift + tsRabbitMQ + tsReport + 1 // = 15
)

type RegionPollConfig struct {
	timeout        time.Duration
	objectStoreUrl string
	region         string
	workers        int
	rabbit         rabbitCreds
}

var AppVersion = "No version provided"

type RegionReport struct {
	// TopAccounts [5]AccountInfo
	TotalConso         int64
	RunDuration        time.Duration
	PolledSuccessfully int
	Polled             int
	Projects           int
	Published          int
	Region             string
}

func (r RegionReport) Publish(gf *graphite.Graphite) {
	gf.SimpleSend(fmt.Sprintf("%v.published", r.Region), fmt.Sprintf("%d", r.Published))
	gf.SimpleSend(fmt.Sprintf("%v.polledsuccessfully", r.Region), fmt.Sprintf("%d", r.PolledSuccessfully))
	gf.SimpleSend(fmt.Sprintf("%v.polled", r.Region), fmt.Sprintf("%d", r.Polled))
	gf.SimpleSend(fmt.Sprintf("%v.projects", r.Region), fmt.Sprintf("%d", r.Projects))
	gf.SimpleSend(fmt.Sprintf("%v.runduration", r.Region), fmt.Sprintf("%d", int(r.RunDuration.Seconds())))
	// We publish total consumption only if we actually managed to poll stuff.
	// TODO: this sucks actually ...
	if float32(r.PolledSuccessfully)/float32(r.Projects) > 0.99 {
		gf.SimpleSend(fmt.Sprintf("%v.totalconso", r.Region), fmt.Sprintf("%d", r.TotalConso))
	}
}

type AccountResult struct {
	ai  AccountInfo
	err error
}

type AccountInfo struct {
	CounterName      string  `json:"counter_name"`       //"storage.objects.size",
	ResourceID       string  `json:"resource_id"`        //"d5bbc7c06c9e479dbb91912c045cdeab",
	MessageID        string  `json:"message_id"`         //"1",
	Timestamp        string  `json:"timestamp"`          // "2013-05-13T14:03:01Z",
	CounterVolume    string  `json:"counter_volume"`     // "0",
	UserID           *string `json:"user_id"`            // null,
	Source           string  `json:"source"`             // "openstack",
	CounterUnit      string  `json:"counter_unit"`       // "B",
	ProjectID        string  `json:"project_id"`         // "d5bbc7c06c9e479dbb91912c045cdeab",
	CounterType      string  `json:"counter_type"`       // "gauge",
	ResourceMetadata *string `json:"ressource_metadata"` // null
	Region           string  `json:"region"`             // "int5"
}

func ReduceAccounts(cfg *RegionPollConfig, in <-chan AccountResult) (RegionReport, error) {
	chunksize := 200
	rr := RegionReport{Region: cfg.region}

	var chunkedAccounts [][]AccountInfo
	var allAccounts []AccountInfo
	for ar := range in {
		rr.Polled++
		if ar.err == nil {
			rr.PolledSuccessfully++
			conso, err := strconv.ParseInt(ar.ai.CounterVolume, 10, 64)
			if err == nil {
				rr.TotalConso += conso
			}
			allAccounts = append(allAccounts, ar.ai)
		}
	}

	log.Infof("Polled %d accounts successfully our of %d", rr.PolledSuccessfully, rr.Polled)
	fits := len(allAccounts) / chunksize
	for i := 0; i < fits; i++ {
		chunkedAccounts = append(chunkedAccounts, allAccounts[i*chunksize:(i+1)*chunksize])
	}
	chunkedAccounts = append(chunkedAccounts, allAccounts[fits*chunksize:])

	if len(chunkedAccounts) == 0 {
		return rr, fmt.Errorf("nothing to publish to rabbitMQ")
	}

	publishChan, confirmChan, err := setupRabbit(cfg.rabbit)
	if err != nil {
		return rr, errors.Wrap(err, "cannot setupRabbit")
	}
	// publishChan, confirmChan, _ := fakeSetupRabbit()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout*tsRabbitMQ/tsSum)
	go func() {
		defer close(publishChan)
		defer cancel()
		for _, a := range chunkedAccounts {
			select {
			case <-ctx.Done():
				return
			case publishChan <- a:
			}
		}
	}()

	for published := range confirmChan {
		rr.Published += published
	}
	log.Infof("published %d accounts out of %d polled successfully", rr.Published, rr.Polled)
	return rr, nil
}

func pollProject(objectStoreURL, region string, project Project, provider *gophercloud.ProviderClient) (AccountInfo, error) {
	accountURL := strings.Join([]string{objectStoreURL, "/v1/AUTH_", project.ID}, "")
	retry := true
	for {
		resp, err := provider.Request("HEAD", accountURL, &gophercloud.RequestOpts{OkCodes: []int{204, 200}})
		if err != nil {
			if retry {
				retry = false
				log.Debug(err, " Retrying")
				<-time.Tick(200 * time.Millisecond)
				continue
			} else {
				log.Error(err)
				return AccountInfo{}, err
			}
		}
		defer resp.Body.Close()
		log.Debug("Fetched account: ", accountURL)
		ai := AccountInfo{
			CounterName:      "storage.objects.size",
			ResourceID:       project.ID,
			MessageID:        uuid.New(),
			Timestamp:        time.Now().Format(time.RFC3339),
			CounterVolume:    resp.Header.Get("x-account-bytes-used"),
			UserID:           nil,
			Source:           "openstack",
			CounterUnit:      "B",
			ProjectID:        project.ID,
			CounterType:      "gauge",
			ResourceMetadata: nil,
			Region:           region,
		}
		return ai, nil
	}
}

// PollWorker is a goroutine that polls swift for projects from chann Project. Exits on context.Done()
func PollWorker(wg *sync.WaitGroup, objectStoreURL, region string, in <-chan Project,
	provider *gophercloud.ProviderClient, out chan AccountResult) {

	defer wg.Done()
	//var errors int
	for project := range in {
		ai, err := pollProject(objectStoreURL, region, project, provider)
		out <- AccountResult{ai, err}
	}
}

// PollRegion polls a region. should run in its own goroutine
func PollRegion(cfg *RegionPollConfig, projects []Project, provider *gophercloud.ProviderClient) (RegionReport, error) {

	projChann := make(chan Project)
	accountResultChann := make(chan AccountResult, len(projects))

	var wg sync.WaitGroup
	for i := 0; i < cfg.workers; i++ {
		wg.Add(1)
		go PollWorker(&wg, cfg.objectStoreUrl, cfg.region, projChann, provider, accountResultChann)
	}

	ctxSwift, cancel := context.WithTimeout(context.Background(), cfg.timeout*tsSwift/tsSum)
	go func() {
		defer close(projChann)
		defer cancel()
		for _, p := range projects {
			select {
			case <-ctxSwift.Done():
				return
			case projChann <- p:
			}
		}
	}()

	go func() {
		// Wait for all workers to finish. If context is canceled, Workers will exit and this will pass.
		wg.Wait()
		close(accountResultChann) // Then we close this chan to terminate the publishing.
	}()

	rr, err := ReduceAccounts(cfg, accountResultChann)

	return rr, err

}

func runOnce(conf config) {
	start := time.Now()

	provider, err := openstack.AuthenticatedClient(conf.Credentials.Openstack.AuthOptions)
	if err != nil {
		log.Fatalf("Failed creating provider: %v", err)
	}

	idClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{})
	if err != nil {
		log.Fatalf("cannot get identity client: %v", err)
	}
	projects, err := getProjects(idClient)
	if err != nil {
		log.Fatalf("Could not get projects: %v", err)
	}

	log.Info(len(projects), " projects retrieved")

	objectStoreURL, err := getEndpoint(idClient, "object-store", conf.Region, "admin")
	if err != nil {
		log.Fatalf("cannot get swift endpoint for region: %v", conf.Region)
	}

	cfg := RegionPollConfig{
		objectStoreUrl: objectStoreURL,
		timeout:        conf.Timeout,
		region:         conf.Region,
		workers:        conf.Workers,
		rabbit:         conf.Credentials.Rabbit,
	}

	report, err := PollRegion(&cfg, projects, provider)
	if err != nil {
		log.Errorf("cannot publish result: %v", err)
	}

	report.RunDuration = time.Since(start)
	report.Projects = len(projects)

	log.Infof("Run Completed in %v. Successfully Polled %v out of %v accounts. Published %d", report.RunDuration.String(), report.PolledSuccessfully, report.Projects, report.Published)
	graphiteClient, err := graphite.NewGraphiteWithMetricPrefix(conf.Graphite.Hostname, conf.Graphite.Port, conf.Graphite.Prefix)
	if err != nil {
		log.Errorf("cannot connect to graphite with hostname: %v port: %v", conf.Graphite.Hostname, conf.Graphite.Port)
		graphiteClient = graphite.NewGraphiteNop(conf.Graphite.Hostname, conf.Graphite.Port)
	}
	report.Publish(graphiteClient)
}

func main() {
	configPath := flag.String("config", "/etc/swift-consometer/", "Path of the configuration file directory.")
	logLevel := flag.String("l", "", "Set log level info|debug|warn|error|panic. Default is info.")
	version := flag.Bool("v", false, "Prints current swift-consometer version and exits.")
	flag.Parse()

	if *version {
		fmt.Println(AppVersion)
		os.Exit(0)
	}

	conf, err := readConfig(*configPath, *logLevel)
	if err != nil {
		log.Fatalf("Failed reading configuration: %v", err)
	}

	go http.ListenAndServe(":8080", http.DefaultServeMux)

	ticker := time.Tick(conf.Timeout)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	// This works around the fact that tickers start after one full interval.
	go runOnce(conf)
	for {
		select {
		case <-ticker:
			go runOnce(conf)
		case <-sig:
			os.Exit(1)
		}
	}
}
