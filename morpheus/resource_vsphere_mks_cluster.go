package morpheus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gomorpheus/morpheus-go-sdk"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	minimumMKSWorkerNodes = 3
	pollIntervalSeconds   = 10

	statusCancelled      = "cancelled"
	statusDenied         = "denied"
	statusDeprovisioned  = "deprovisioned"
	statusDeprovisioning = "deprovisioning"
	statusFailed         = "failed"
	statusOk             = "ok"
	statusPending        = "pending"
	statusPendingRemoval = "pendingRemoval"
	statusProvisioning   = "provisioning"
	statusProvisioned    = "provisioned"
	statusRemoved        = "removed"
	statusRemoving       = "removing"
	statusRunning        = "running"
	statusStarting       = "starting"
	statusStopping       = "stopping"
	statusSuspended      = "suspended"
	statusSyncing        = "syncing"
	statusWarning        = "warning"
)

func validateCountDiagFunc(i interface{}, _ cty.Path) diag.Diagnostics {
	count := i.(int)
	if count < minimumMKSWorkerNodes {
		return diag.Errorf("count must be a minimum of %d, count is %d", minimumMKSWorkerNodes, count)
	}

	return nil
}

func defaultCountFunc() (interface{}, error) {
	return minimumMKSWorkerNodes, nil
}

func resourceVsphereMKSCluster() *schema.Resource {
	return &schema.Resource{
		Description:   "Provides an Morpheus Kubernetes Service (MKS) cluster on VMware vSphere resource",
		CreateContext: resourceVsphereMKSClusterCreate,
		ReadContext:   resourceVsphereMKSClusterRead,
		UpdateContext: resourceVsphereMKSClusterUpdate,
		DeleteContext: resourceVsphereMKSClusterDelete,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(45 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(45 * time.Minute),
			Delete: schema.DefaultTimeout(45 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "The ID of the cluster",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"api_endpoint": {
				Description: "The API URL of the cluster",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"kubernetes_version": {
				Description: "The Kubernetes version of the cluster",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"name": {
				Description: "The name of the cluster",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"resource_prefix": {
				Description: "The prefix used for the virtual machine name of the master and worker nodes",
				Type:        schema.TypeString,
				ForceNew:    true,
				Optional:    true,
				Computed:    true,
			},
			"hostname_prefix": {
				Description: "The prefix used for the guest operating system hostname of the master and worker nodes",
				Type:        schema.TypeString,
				ForceNew:    true,
				Optional:    true,
				Computed:    true,
			},
			"description": {
				Description: "The user friendly description of the cluster",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"cloud_id": {
				Description: "The ID of the cloud associated with the cluster",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Required:    true,
			},
			"group_id": {
				Description: "The ID of the group associated with the cluster",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Required:    true,
			},
			"cluster_layout_id": {
				Description: "The ID of the cluster layout to provision the cluster from",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Required:    true,
			},
			"api_proxy_id": {
				Description: "The ID of the api proxy associated with the cluster",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Optional:    true,
			},
			// AWAITING API Support
			// "visibility": {
			//	Type:         schema.TypeString,
			//	Description:  "The visibility of the cluster (public or private)",
			//	Required:     true,
			//	ValidateFunc: validation.StringInSlice([]string{"public", "private"}, false),
			//},
			"pod_cidr": {
				Description: "The cluster pod cidr (default - 172.20.0.0/16)",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "172.20.0.0/16",
			},
			"service_cidr": {
				Description: "The cluster service cidr (default - 172.30.0.0/16)",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "172.30.0.0/16",
			},
			// AWAITING API Support
			//"labels": {
			//	Type:        schema.TypeList,
			//	Description: "The list of labels to add to the cluster",
			//	Optional:    true,
			//	Elem: &schema.Schema{
			//		Type: schema.TypeString,
			//	},
			//	Computed: true,
			//},
			"cluster_repo_account_id": {
				Description: "The ID of the cluster repo account associated with the cluster",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Optional:    true,
			},
			"workflow_id": {
				Description: "The ID of the provisioning workflow to execute",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Optional:    true,
			},
			"master_node_pool": {
				Type:        schema.TypeList,
				Description: "Master node pool configuration",
				ForceNew:    true,
				Optional:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"plan_id": {
							Description: "The ID of the service plan associated with the master nodes in the cluster",
							Type:        schema.TypeInt,
							ForceNew:    true,
							Required:    true,
						},
						"resource_pool_id": {
							Description: "The ID of the resource pool to provision the cluster master nodes to",
							Type:        schema.TypeInt,
							ForceNew:    true,
							Optional:    true,
							Computed:    true,
						},
						"storage_volume": {
							Description: "The storage volumes to create for the cluster master nodes",
							Type:        schema.TypeList,
							ForceNew:    true,
							Optional:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Description: "The storage volume uuid",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"root": {
										Description: "Whether the volume is the root volume of the instance",
										Type:        schema.TypeBool,
										ForceNew:    true,
										Required:    true,
									},
									"name": {
										Description: "The name of the volume",
										Type:        schema.TypeString,
										ForceNew:    true,
										Required:    true,
									},
									"size": {
										Description: "The size of the volume in GB",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									"storage_type": {
										Description: "The storage volume type ID",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									"datastore_id": {
										Description: "The ID of the datastore",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
								},
							},
						},
						"network_interface": {
							Description: "The network interfaces to create for the cluster master nodes",
							Type:        schema.TypeList,
							Optional:    true,
							ForceNew:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"network_id": {
										Description: "The ID of the network to assign the network interface to",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									/* AWAITING API Support for the master node pool for consistency
									"network_interface_type_id": {
										Description: "The id of the network interface type",
										Type:        schema.TypeInt,
										Optional:    true,
									},
									*/
								},
							},
						},
						"tags": {
							Description: "Tags to assign to the cluster master nodes",
							Type:        schema.TypeMap,
							ForceNew:    false,
							Optional:    true,
							Computed:    true,
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
			"worker_node_pool": {
				Type:        schema.TypeList,
				Description: "Worker node pool configuration",
				Optional:    true,
				ForceNew:    false,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"count": {
							Description:      "The number of worker nodes",
							Type:             schema.TypeInt,
							ForceNew:         false,
							Required:         true,
							DefaultFunc:      defaultCountFunc,
							ValidateDiagFunc: validateCountDiagFunc,
						},
						"plan_id": {
							Description: "The ID of the service plan associated with the worker nodes in the cluster",
							Type:        schema.TypeInt,
							ForceNew:    true,
							Required:    true,
						},
						"resource_pool_id": {
							Description: "The ID of the resource pool to provision the cluster worker nodes to",
							Type:        schema.TypeInt,
							ForceNew:    true,
							Optional:    true,
							Computed:    true,
						},
						"tags": {
							Description: "Tags to assign to the cluster worker nodes",
							Type:        schema.TypeMap,
							ForceNew:    false,
							Optional:    true,
							Computed:    true,
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"storage_volume": {
							Description: "The storage volumes to create for the cluster worker nodes",
							Type:        schema.TypeList,
							ForceNew:    true,
							Optional:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Description: "The storage volume uuid",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"root": {
										Description: "Whether the volume is the root volume of the instance",
										Type:        schema.TypeBool,
										ForceNew:    true,
										Required:    true,
									},
									"name": {
										Description: "The name of the volume",
										Type:        schema.TypeString,
										ForceNew:    true,
										Required:    true,
									},
									"size": {
										Description: "The size of the volume in GB",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									"storage_type": {
										Description: "The storage volume type ID",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									"datastore_id": {
										Description: "The ID of the datastore",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
								},
							},
						},
						"network_interface": {
							Description: "The network interfaces to create for the cluster worker nodes",
							Type:        schema.TypeList,
							ForceNew:    true,
							Optional:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"network_id": {
										Description: "The ID of the network to attach the interface to",
										Type:        schema.TypeInt,
										ForceNew:    true,
										Required:    true,
									},
									/* AWAITING API Support for the master node pool for consistency
									"network_interface_type_id": {
										Description: "The id of the network interface type",
										Type:        schema.TypeInt,
										Optional:    true,
									},
									*/
								},
							},
						},
					},
				},
			},
		},
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

func getClusterWorkers(client *morpheus.Client, clusterId int64) ([]morpheus.ClusterWorker, error) {
	resp, err := client.ListClusterWorkers(clusterId, &morpheus.Request{})
	if err != nil {
		log.Printf("API FAILURE - Error in listing cluster worker nodes: %s - %s", resp, err)
		return nil, err
	}

	var workerResp morpheus.ListClusterWorkersResults
	if err := json.Unmarshal(resp.Body, &workerResp); err != nil {
		return nil, err
	}

	// Sort the workers by date created to avoid naming problems i.e. worker-1-1
	sort.Slice(*workerResp.Workers, func(i, j int) bool {
		return (*workerResp.Workers)[i].DateCreated.Unix() < (*workerResp.Workers)[j].DateCreated.Unix()
	})

	return *workerResp.Workers, nil
}

func filterClusterWorkersByStatus(workers []morpheus.ClusterWorker, status string) []morpheus.ClusterWorker {
	var filteredWorkers []morpheus.ClusterWorker

	for _, worker := range workers {
		if worker.Status == status {
			filteredWorkers = append(filteredWorkers, worker)
		}
	}

	return filteredWorkers
}

func filterOutClusterWorkersByStatus(workers []morpheus.ClusterWorker, status string) []morpheus.ClusterWorker {
	var filteredWorkers []morpheus.ClusterWorker

	for _, worker := range workers {
		if worker.Status != status {
			filteredWorkers = append(filteredWorkers, worker)
		}
	}

	return filteredWorkers
}

func resourceVsphereMKSClusterCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*morpheus.Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	clusterPayload := map[string]interface{}{}
	clusterPayload["name"] = d.Get("name").(string)
	clusterPayload["type"] = "kubernetes-cluster"
	clusterPayload["autoRecoverPowerState"] = false
	clusterPayload["cloud"] = map[string]interface{}{
		"id": d.Get("cloud_id").(int),
	}

	// Group
	clusterPayload["group"] = map[string]interface{}{
		"id": d.Get("group_id").(int),
	}

	// Labels - AWAITING API support
	//if d.Get("labels") != nil {
	//	clusterPayload["labels"] = d.Get("labels")
	//}

	// Description
	if d.Get("description") != nil {
		clusterPayload["description"] = d.Get("description").(string)
	}

	// Cluster Layout
	clusterPayload["layout"] = map[string]interface{}{
		"id": d.Get("cluster_layout_id").(int),
	}

	// Workflow
	clusterPayload["taskSetId"] = d.Get("workflow_id").(int)

	masterpool := d.Get("master_node_pool").([]interface{})[0].(map[string]interface{})
	workerpool := d.Get("worker_node_pool").([]interface{})[0].(map[string]interface{})

	serverPayload := map[string]interface{}{}
	serverPayload["config"] = map[string]interface{}{
		"podCidr":            d.Get("pod_cidr").(string),
		"serviceCidr":        d.Get("service_cidr").(string),
		"resourcePoolId":     masterpool["resource_pool_id"],
		"nodeCount":          workerpool["count"],
		"defaultRepoAccount": d.Get("cluster_repo_account_id").(int),
	}
	serverPayload["nodeCount"] = workerpool["count"]
	// serverPayload["visibility"] = d.Get("visibility").(string)
	serverPayload["volumes"] = parseStorageVolumes(masterpool["storage_volume"].([]interface{}))
	serverPayload["networkInterfaces"] = parseMasterNetworkInterfaces(masterpool["network_interface"].([]interface{}))

	if masterpool["tags"] != nil {
		serverPayload["tags"] = parseTags(masterpool["tags"].(map[string]interface{}))
	}

	serverPayload["plan"] = map[string]interface{}{
		"id": masterpool["plan_id"],
	}

	serverPayload["apiProxy"] = map[string]interface{}{
		"id": d.Get("api_proxy_id").(int),
	}

	serverPayload["hostname"] = d.Get("hostname_prefix").(string)
	serverPayload["name"] = d.Get("resource_prefix").(string)

	workerPayload := map[string]interface{}{}
	workerPayload["apiProxy"] = map[string]interface{}{
		"id": d.Get("api_proxy_id").(int),
	}
	workerPayload["volumes"] = parseStorageVolumes(workerpool["storage_volume"].([]interface{}))
	workerPayload["networkInterfaces"] = parseWorkerNetworkInterfaces(workerpool["network_interface"].([]interface{}))
	workerPayload["config"] = map[string]interface{}{
		"resourcePoolId": workerpool["resource_pool_id"],
	}
	workerServerPayload := map[string]interface{}{
		"plan": map[string]interface{}{
			"id": workerpool["plan_id"],
		},
	}

	if workerpool["tags"] != nil {
		workerPayload["tags"] = parseTags(workerpool["tags"].(map[string]interface{}))
	}
	workerPayload["server"] = workerServerPayload

	clusterPayload["worker"] = workerPayload
	clusterPayload["server"] = serverPayload

	req := &morpheus.Request{Body: map[string]interface{}{
		"cluster": clusterPayload,
	}}

	resp, err := client.CreateCluster(req)
	if err != nil {
		log.Printf("API FAILURE: %s - %s", resp, err)
		return diag.FromErr(err)
	}
	log.Printf("API RESPONSE: %s", resp)
	result := resp.Result.(*morpheus.CreateClusterResult)
	cluster := result.Cluster
	clusterStatus := statusProvisioning

	stateConf := &resource.StateChangeConf{
		Pending: []string{statusProvisioning, statusStarting, statusStopping, statusPending, statusSyncing},
		Target:  []string{statusRunning, statusFailed, statusWarning, statusDenied, statusCancelled, statusSuspended, statusOk},
		Refresh: func() (interface{}, string, error) {
			clusterDetails, err := client.GetCluster(cluster.ID, &morpheus.Request{})
			if err != nil {
				return "", "", err
			}
			log.Printf("API RESPONSE: %s", clusterDetails)
			result := clusterDetails.Result.(*morpheus.GetClusterResult)
			cluster := result.Cluster
			clusterStatus = cluster.Status
			if clusterStatus == statusFailed {
				hostsDetails, err := client.ListHosts(&morpheus.Request{
					QueryParams: map[string]string{
						"clusterId": strconv.Itoa(int(cluster.ID)),
					},
				})
				if err != nil {
					log.Printf("API FAILURE: %s - %s", resp, err)
				}
				hostsResults := hostsDetails.Result.(*morpheus.ListHostsResult)
				for _, host := range *hostsResults.Hosts {
					// Override the cluster status if the worker nodes are still provisioning
					// to avoid a false failure while the cluster is still being deployed. This is
					// a workaround that has been fixed in 8.0.4 but has been added for legacy support.
					if host.Status == statusProvisioning {
						clusterStatus = statusProvisioning
					}
				}
			}
			// Added an arbitrary wait period for cluster refresh.
			// This should probably trigger a cluster refresh and then poll
			// the cluster to reach a definitive state.
			if clusterStatus == statusFailed {
				time.Sleep(3 * time.Minute)
				clusterStatus = statusOk
			}

			return result, clusterStatus, nil
		},
		Timeout:      3 * time.Hour,
		MinTimeout:   1 * time.Minute,
		Delay:        3 * time.Minute,
		PollInterval: 1 * time.Minute,
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error creating cluster: %s", err)
	}

	// Successfully created resource, now set id
	d.SetId(int64ToString(cluster.ID))
	resourceVsphereMKSClusterRead(ctx, d, meta)

	// Fail the cluster deployment if the cluster status is in a failed state
	if clusterStatus == statusFailed {
		return diag.Errorf("error creating cluster: failed to create cluster")
	}
	return diags
}

func resourceVsphereMKSClusterRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*morpheus.Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	id := d.Id()
	name := d.Get("name").(string)

	// lookup by name if we do not have an id yet
	var resp *morpheus.Response
	var err error
	if id == "" && name != "" {
		resp, err = client.FindClusterByName(name)
	} else if id != "" {
		resp, err = client.GetCluster(toInt64(id), &morpheus.Request{})
	} else {
		return diag.Errorf("Cluster cannot be read without name or id")
	}
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			log.Printf("API 404: %s - %s", resp, err)
			log.Printf("Forcing recreation of resource")
			d.SetId("")
			return diags
		} else {
			log.Printf("API FAILURE: %s - %s", resp, err)
			return diag.FromErr(err)
		}
	}

	// store resource data
	result := resp.Result.(*morpheus.GetClusterResult)
	cluster := result.Cluster
	if cluster == nil {
		return diag.Errorf("Cluster not found in response data.") // should not happen
	}

	d.SetId(int64ToString(cluster.ID))
	d.Set("name", cluster.Name)
	d.Set("description", cluster.Description)
	d.Set("cloud_id", cluster.Zone.Id)
	d.Set("group_id", cluster.Site.Id)
	d.Set("cluster_layout_id", cluster.Layout.Id)
	// d.Set("visibility", cluster.Visibility)
	d.Set("kubernetes_version", cluster.ServiceVersion)
	d.Set("api_endpoint", cluster.ServiceUrl)

	workers, err := getClusterWorkers(client, cluster.ID)
	if err != nil {
		return diag.FromErr(err)
	}
	workers = filterOutClusterWorkersByStatus(workers, statusDeprovisioning)
	worker := workers[0]

	tags := make(map[string]interface{}, len(worker.Tags))
	for _, i := range worker.Tags {
		tag := i.(map[string]interface{})
		tags[tag["name"].(string)] = tag["value"]
	}

	var volumes []map[string]interface{}
	for _, v := range worker.Volumes {
		sizeGB := v.MaxStorage / (1 << 30)
		volume := map[string]interface{}{
			"root":         v.RootVolume,
			"name":         v.Name,
			"datastore_id": v.DatastoreId,
			"storage_type": v.TypeId,
			"size":         sizeGB,
		}
		volumes = append(volumes, volume)
	}

	var networks []map[string]interface{}
	for _, v := range worker.Interfaces {
		network := map[string]interface{}{
			"network_id": v.Network.ID,
		}
		networks = append(networks, network)
	}

	workerNodePool := []interface{}{
		map[string]interface{}{
			"count":             len(workers),
			"plan_id":           worker.Plan.ID,
			"resource_pool_id":  worker.ResourcePoolId,
			"tags":              tags,
			"storage_volume":    volumes,
			"network_interface": networks,
		},
	}

	d.Set("worker_node_pool", workerNodePool)

	return diags
}

func doClusterWorkerAdd(ctx context.Context, client *morpheus.Client, clusterId int64, nodeCount int, d *schema.ResourceData) error {
	workerpool := d.Get("worker_node_pool").([]interface{})[0].(map[string]interface{})

	workers, err := getClusterWorkers(client, clusterId)
	if err != nil {
		return err
	}
	worker := workers[0]
	desiredWorkerCount := len(workers) + nodeCount

	serverPayload := map[string]interface{}{}
	serverPayload["config"] = map[string]interface{}{
		"podCidr":            d.Get("pod_cidr").(string),
		"serviceCidr":        d.Get("service_cidr").(string),
		"nodeCount":          workerpool["count"], // Might need to go in serverPayload.server
		"resourcePoolId":     workerpool["resource_pool_id"],
		"defaultRepoAccount": d.Get("cluster_repo_account_id").(int),
	}

	// We will let Morpheus set the name for us.

	serverPayload["serverType"] = map[string]interface{}{
		"id": worker.ComputeServerType.ID,
	}
	serverPayload["cloud"] = map[string]interface{}{
		"id": d.Get("cloud_id").(int),
	}
	serverPayload["plan"] = map[string]interface{}{
		"id": workerpool["plan_id"],
	}

	serverPayload["volumes"] = parseStorageVolumes(workerpool["storage_volume"].([]interface{}))
	serverPayload["networkInterfaces"] = parseWorkerNetworkInterfacesForWorkerPayload(workerpool["network_interface"].([]interface{}))
	serverPayload["nodeCount"] = nodeCount
	serverPayload["tags"] = parseTags(workerpool["tags"].(map[string]interface{}))

	// NOTE: Not needed from Morpheus 8.05 onward
	serverPayload["server"] = map[string]interface{}{
		"network": map[string]interface{}{},
	}

	req := &morpheus.Request{Body: map[string]interface{}{
		"server": serverPayload,
	}}

	resp, err := client.AddClusterWorker(clusterId, req)
	if err != nil {
		log.Printf("API FAILURE - Error in creating cluster worker node(s): %s - %s", resp, err)

		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending: []string{statusProvisioning},
		Target:  []string{statusProvisioned},
		Refresh: func() (interface{}, string, error) {
			log.Printf("Waiting for all cluster worker nodes to be provisioned...")

			workers, err := getClusterWorkers(client, clusterId)
			if err != nil {
				return "", "", err
			}

			failedWorkers := filterClusterWorkersByStatus(workers, statusFailed)
			if len(failedWorkers) > 0 {
				return "", "", fmt.Errorf("failed to provision all cluster worker nodes")
			}

			provisionedWorkers := filterClusterWorkersByStatus(workers, statusProvisioned)
			if len(provisionedWorkers) == desiredWorkerCount {
				return "", statusProvisioned, nil
			}

			return "", statusProvisioning, nil
		},
		Timeout:      30 * time.Minute,
		MinTimeout:   1 * time.Minute,
		Delay:        1 * time.Minute,
		PollInterval: pollIntervalSeconds * time.Second,
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	return nil
}

func doClusterWorkerDelete(ctx context.Context, client *morpheus.Client, clusterId int64, nodeCount int) error {
	workers, err := getClusterWorkers(client, clusterId)
	if err != nil {
		return err
	}
	workers = filterOutClusterWorkersByStatus(workers, statusDeprovisioning)

	deleteWorkers := workers[len(workers)+nodeCount:]
	for _, worker := range deleteWorkers {
		resp, err := client.DeleteClusterWorker(clusterId, worker.ID, &morpheus.Request{})
		if err != nil {
			log.Printf("API FAILURE - Error in deleting cluster worker node: %s - %s", resp, err)

			return err
		}
	}

	stateConf := &resource.StateChangeConf{
		Pending: []string{statusDeprovisioning},
		Target:  []string{statusDeprovisioned},
		Refresh: func() (interface{}, string, error) {
			log.Printf("Waiting for cluster worker nodes to be deprovisioned...")

			workers, err := getClusterWorkers(client, clusterId)
			if err != nil {
				return "", "", err
			}

			deprovisioningWorkers := filterClusterWorkersByStatus(workers, statusDeprovisioning)
			if len(deprovisioningWorkers) == 0 {
				return "", statusDeprovisioned, nil
			}

			return "", statusDeprovisioning, nil
		},
		Timeout:      30 * time.Minute,
		MinTimeout:   1 * time.Minute,
		Delay:        1 * time.Minute,
		PollInterval: pollIntervalSeconds * time.Second,
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	return nil
}

func resourceVsphereMKSClusterUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*morpheus.Client)
	clusterId := toInt64(d.Id())

	// First check for changes in worker node pool
	if d.HasChange("worker_node_pool") {
		o, n := d.GetChange("worker_node_pool")
		oldValues, ok := o.([]interface{})[0].(map[string]interface{})
		if !ok {
			return diag.Errorf("failed to get old worker_node_pool.count")
		}

		oldCount, ok := oldValues["count"].(int)
		if !ok {
			return diag.Errorf("failed to get old worker_node_pool.count as int")
		}

		newValues, ok := n.([]interface{})[0].(map[string]interface{})
		if !ok {
			return diag.Errorf("failed to get new worker_node_pool.count")
		}

		newCount, ok := newValues["count"].(int)
		if !ok {
			return diag.Errorf("failed to get new worker_node_pool.count as int")
		}

		if newCount != oldCount {
			countDelta := newCount - oldCount

			if countDelta > 0 {
				err := doClusterWorkerAdd(ctx, client, clusterId, countDelta, d)
				if err != nil {
					return diag.Errorf("error adding cluster worker node(s): %s", err)
				}
			} else {
				err := doClusterWorkerDelete(ctx, client, clusterId, countDelta)
				if err != nil {
					return diag.Errorf("error deleting cluster worker node(s): %s", err)
				}
			}
		}
	}

	clusterPayload := map[string]interface{}{}

	if d.HasChange("name") {
		clusterPayload["name"] = d.Get("name").(string)
	}

	if d.HasChange("description") {
		clusterPayload["description"] = d.Get("description").(string)
	}

	if len(clusterPayload) > 0 {
		req := &morpheus.Request{Body: map[string]interface{}{
			"cluster": clusterPayload,
		}}

		resp, err := client.UpdateCluster(clusterId, req)
		if err != nil {
			log.Printf("API FAILURE: %s - %s", resp, err)
			return diag.FromErr(err)
		}
		log.Printf("API RESPONSE: %s", resp)
	}

	return resourceVsphereMKSClusterRead(ctx, d, meta)
}

func resourceVsphereMKSClusterDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*morpheus.Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	id := d.Id()
	req := &morpheus.Request{
		QueryParams: map[string]string{
			"removeInstances": "on",
			"removeResources": "on",
		},
	}
	if USE_FORCE {
		req.QueryParams["force"] = "true"
	}
	resp, err := client.DeleteCluster(toInt64(id), req)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			log.Printf("API 404: %s - %s", resp, err)
			return diag.FromErr(err)
		} else {
			log.Printf("API FAILURE: %s - %s", resp, err)
			return diag.FromErr(err)
		}
	}
	log.Printf("API RESPONSE: %s", resp)

	stateConf := &resource.StateChangeConf{
		Pending: []string{statusRemoving, statusPendingRemoval, statusStopping, statusPending, statusWarning, statusDeprovisioning},
		Target:  []string{statusRemoved},
		Refresh: func() (interface{}, string, error) {
			clusterDetails, err := client.GetCluster(toInt64(id), &morpheus.Request{})
			if clusterDetails.StatusCode == 404 {
				return "", "removed", nil
			}
			if err != nil {
				return "", "", err
			}
			result := clusterDetails.Result.(*morpheus.GetClusterResult)
			cluster := result.Cluster
			return result, cluster.Status, nil
		},
		Timeout:      30 * time.Minute,
		MinTimeout:   1 * time.Minute,
		Delay:        1 * time.Minute,
		PollInterval: 30 * time.Second,
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Errorf("error deleting cluster: %s", err)
	}

	d.SetId("")
	return diags
}

func parseMasterNetworkInterfaces(variables []interface{}) []map[string]interface{} {
	// Master network interfaces passes a string including an integer (network-1) directly passed via the API
	var networkInterfaces []map[string]interface{}

	for i := 0; i < len(variables); i++ {
		networkInterface := make(map[string]interface{})
		for k, v := range variables[i].(map[string]interface{}) {
			switch k {
			case "network_id":
				networkId := make(map[string]interface{})
				networkId["id"] = fmt.Sprintf("network-%d", v.(int))
				networkInterface["network"] = networkId
			}
		}
		networkInterfaces = append(networkInterfaces, networkInterface)
	}
	return networkInterfaces
}

func parseWorkerNetworkInterfaces(variables []interface{}) []map[string]interface{} {
	// Worker network interfaces passes an integer (1) directly passed via the API
	var networkInterfaces []map[string]interface{}

	for i := 0; i < len(variables); i++ {
		networkInterface := make(map[string]interface{})
		for k, v := range variables[i].(map[string]interface{}) {
			switch k {
			case "network_id":
				networkId := make(map[string]interface{})
				networkId["id"] = v.(int)
				networkInterface["network"] = networkId
			}
		}
		networkInterfaces = append(networkInterfaces, networkInterface)
	}
	return networkInterfaces
}

func parseWorkerNetworkInterfacesForWorkerPayload(variables []interface{}) []map[string]interface{} {
	// For a payload for Add Workers API, it expects the ID of the network interface in the string form "network-{id}"
	var networkInterfaces []map[string]interface{}

	for i := 0; i < len(variables); i++ {
		networkInterface := make(map[string]interface{})
		for k, v := range variables[i].(map[string]interface{}) {
			switch k {
			case "network_id":
				networkId := make(map[string]interface{})
				networkId["id"] = fmt.Sprintf("network-%d", v.(int))
				networkInterface["network"] = networkId
			}
		}
		networkInterfaces = append(networkInterfaces, networkInterface)
	}
	return networkInterfaces
}

func parseTags(variables map[string]interface{}) []map[string]interface{} {
	var tags []map[string]interface{}
	for key, value := range variables {
		tag := make(map[string]interface{})
		tag["name"] = key
		tag["value"] = value.(string)
		tags = append(tags, tag)
	}
	return tags
}
