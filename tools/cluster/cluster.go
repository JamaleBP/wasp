package cluster

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"text/template"

	waspapi "github.com/iotaledger/wasp/packages/apilib"
)

type SmartContractFinalConfig struct {
	Address          string   `json:"address"`
	Color            string   `json:"color"`
	Description      string   `json:"description"`
	ProgramHash      string   `json:"program_hash"`
	CommitteeNodes   []int    `json:"committee_nodes"`
	AccessNodes      []int    `json:"access_nodes,omitempty"`
	OwnerIndexUtxodb int      `json:"owner_index_utxodb"`
	DKShares         []string `json:"dkshares"` // [node index]
}

type SmartContractInitData struct {
	Description    string `json:"description"`
	CommitteeNodes []int  `json:"committee_nodes"`
	AccessNodes    []int  `json:"access_nodes,omitempty"`
	Quorum         int    `json:"quorum"`
}

type ClusterConfig struct {
	Nodes []struct {
		NetAddress  string `json:"net_address"`
		ApiPort     int    `json:"api_port"`
		PeeringPort int    `json:"peering_port"`
	} `json:"nodes"`
	Goshimmer      string                  `json:"goshimmer"`
	SmartContracts []SmartContractInitData `json:"smart_contracts"`
}

type Cluster struct {
	Config              *ClusterConfig
	SmartContractConfig []SmartContractFinalConfig
	ConfigPath          string // where the cluster configuration is stored - read only
	DataPath            string // where the cluster's volatile data lives
	Started             bool
	cmds                []*exec.Cmd
}

func readConfig(configPath string) (*ClusterConfig, error) {
	data, err := ioutil.ReadFile(path.Join(configPath, "cluster.json"))
	if err != nil {
		return nil, err
	}

	config := &ClusterConfig{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func New(configPath string, dataPath string) (*Cluster, error) {
	config, err := readConfig(configPath)
	if err != nil {
		return nil, err
	}
	return &Cluster{
		Config:     config,
		ConfigPath: configPath,
		DataPath:   dataPath,
		cmds:       make([]*exec.Cmd, 0),
	}, nil
}

func (cluster *Cluster) readKeysConfig() ([]SmartContractFinalConfig, error) {
	data, err := ioutil.ReadFile(cluster.ConfigKeysPath())
	if err != nil {
		return nil, err
	}

	config := make([]SmartContractFinalConfig, 0)
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (cluster *Cluster) JoinConfigPath(s string) string {
	return path.Join(cluster.ConfigPath, s)
}

func (cluster *Cluster) ConfigTemplatePath() string {
	return cluster.JoinConfigPath("wasp-config-template.json")
}

func (cluster *Cluster) ConfigKeysPath() string {
	return cluster.JoinConfigPath("keys.json")
}

func (cluster *Cluster) NodeDataPath(i int) string {
	return path.Join(cluster.DataPath, strconv.Itoa(i))
}

func (cluster *Cluster) JoinNodeDataPath(i int, s string) string {
	return path.Join(cluster.NodeDataPath(i), s)
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

// Init creates in DataPath a directory with config.json for each node
func (cluster *Cluster) Init(resetDataPath bool) error {
	exists, err := fileExists(cluster.DataPath)
	if err != nil {
		return err
	}
	if exists {
		if !resetDataPath {
			return errors.New(fmt.Sprintf("%s directory exists", cluster.DataPath))
		}
		err = os.RemoveAll(cluster.DataPath)
		if err != nil {
			return err
		}
	}

	configTmpl, err := template.ParseFiles(cluster.ConfigTemplatePath())
	if err != nil {
		return err
	}

	for i, nodeConfig := range cluster.Config.Nodes {
		nodePath := cluster.NodeDataPath(i)
		fmt.Printf("Initializing node configuration at %s\n", nodePath)

		err := os.MkdirAll(nodePath, os.ModePerm)
		if err != nil {
			return err
		}

		f, err := os.Create(cluster.JoinNodeDataPath(i, "config.json"))
		if err != nil {
			return err
		}
		defer f.Close()
		err = configTmpl.Execute(f, &nodeConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func logNode(i int, scanner *bufio.Scanner, initString string, initOk chan bool) {
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if !found && strings.Contains(line, initString) {
			initOk <- true
			found = true
		}
		fmt.Printf("[wasp %d] %s\n", i, line)
	}
}

// Start launches all wasp nodes in the cluster, each running in its own directory
func (cluster *Cluster) Start() error {
	exists, err := fileExists(cluster.DataPath)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New(fmt.Sprintf("Data path %s does not exist", cluster.DataPath))
	}

	err = cluster.start()
	if err != nil {
		return err
	}

	keysExist, err := cluster.readKeysAndData()
	if err != nil {
		return err
	}
	if keysExist {
		err = cluster.importKeys()
		if err != nil {
			return err
		}

	} else {
		fmt.Printf("[cluster] keys.json does not exist\n")
	}
	cluster.Started = true
	return nil
}

func (cluster *Cluster) start() error {
	fmt.Printf("[cluster] starting %d Wasp nodes...\n", len(cluster.Config.Nodes))

	initOk := make(chan bool, len(cluster.Config.Nodes))

	for i, _ := range cluster.Config.Nodes {
		cmd := exec.Command("wasp")
		cmd.Dir = cluster.NodeDataPath(i)
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(pipe)
		err = cmd.Start()
		if err != nil {
			return err
		}
		cluster.cmds = append(cluster.cmds, cmd)

		go logNode(i, scanner, "WebAPI started", initOk)
	}

	for i := 0; i < len(cluster.Config.Nodes); i++ {
		<-initOk
	}
	fmt.Printf("[cluster] started %d Wasp nodes\n", len(cluster.Config.Nodes))
	return nil
}

func (cluster *Cluster) readKeysAndData() (bool, error) {
	exists, err := fileExists(cluster.ConfigKeysPath())
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	fmt.Printf("[cluster] loading keys and smart contract data from %s\n", cluster.ConfigKeysPath())
	cluster.SmartContractConfig, err = cluster.readKeysConfig()
	return true, nil
}

func (cluster *Cluster) importKeys() error {
	for _, scKeys := range cluster.SmartContractConfig {
		fmt.Printf("[cluster] Importing DKShares for address %s...\n", scKeys.Address)
		for nodeIndex, dks := range scKeys.DKShares {
			url := fmt.Sprintf("%s:%d", cluster.Config.Nodes[nodeIndex].NetAddress, cluster.Config.Nodes[nodeIndex].ApiPort)
			err := waspapi.ImportDKShare(url, dks)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Stop sends an interrupt signal to all nodes and waits for them to exit
func (cluster *Cluster) Stop() {
	for _, node := range cluster.Config.Nodes {
		url := fmt.Sprintf("%s:%d", node.NetAddress, node.ApiPort)
		fmt.Printf("[cluster] Sending shutdown to %s\n", url)
		err := waspapi.Shutdown(url)
		if err != nil {
			fmt.Println(err)
		}
	}
	cluster.Wait()
}

// Wait blocks until all nodes exit
func (cluster *Cluster) Wait() {
	for _, cmd := range cluster.cmds {
		err := cmd.Wait()
		if err != nil {
			fmt.Println(err)
		}
	}
}

func (cluster *Cluster) ApiHosts() []string {
	hosts := make([]string, 0)
	for _, node := range cluster.Config.Nodes {
		url := fmt.Sprintf("%s:%d", node.NetAddress, node.ApiPort)
		hosts = append(hosts, url)
	}
	return hosts
}

func (cluster *Cluster) Committee(sc *SmartContractInitData) ([]string, error) {
	committee := make([]string, 0)
	for _, i := range sc.CommitteeNodes {
		if i < 0 || i > len(cluster.Config.Nodes)-1 {
			return nil, errors.New(fmt.Sprintf("Node index out of bounds in smart contract committee configuration: %d", i))
		}
		url := fmt.Sprintf("%s:%d", cluster.Config.Nodes[i].NetAddress, cluster.Config.Nodes[i].ApiPort)
		committee = append(committee, url)
	}
	return committee, nil
}