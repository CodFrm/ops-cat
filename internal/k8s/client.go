package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

type NodeInfo struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Roles   []string `json:"roles"`
	Version string   `json:"version"`
	CPU     string   `json:"cpu"`
	Memory  string   `json:"memory"`
	OS      string   `json:"os"`
	Arch    string   `json:"arch"`
}

type NamespaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ClusterInfo struct {
	Version    string          `json:"version"`
	Platform   string          `json:"platform"`
	Nodes      []NodeInfo      `json:"nodes"`
	Namespaces []NamespaceInfo `json:"namespaces"`
}

type ClientOption func(*clientOptions)

type clientOptions struct {
	dial        func(ctx context.Context, network, address string) (net.Conn, error)
	contextName string
	timeout     *time.Duration
}

func WithDial(dial func(ctx context.Context, network, address string) (net.Conn, error)) ClientOption {
	return func(opts *clientOptions) {
		opts.dial = dial
	}
}

func WithContext(contextName string) ClientOption {
	return func(opts *clientOptions) {
		opts.contextName = contextName
	}
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(opts *clientOptions) {
		opts.timeout = &timeout
	}
}

func GetClusterInfo(ctx context.Context, kubeconfig string, opts ...ClientOption) (*ClusterInfo, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	info := &ClusterInfo{}

	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get server version: %w", err)
	}
	info.Version = serverVersion.GitVersion
	info.Platform = serverVersion.Platform

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	for _, node := range nodeList.Items {
		ni := NodeInfo{
			Name:    node.Name,
			Version: node.Status.NodeInfo.KubeletVersion,
			OS:      node.Status.NodeInfo.OperatingSystem,
			Arch:    node.Status.NodeInfo.Architecture,
			CPU:     node.Status.Capacity.Cpu().String(),
			Memory:  node.Status.Capacity.Memory().String(),
		}
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				ni.Status = string(cond.Status)
			}
		}
		ni.Roles = getNodeRoles(&node)
		info.Nodes = append(info.Nodes, ni)
	}

	nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	for _, ns := range nsList.Items {
		info.Namespaces = append(info.Namespaces, NamespaceInfo{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
		})
	}

	return info, nil
}

func buildClient(kubeconfig string, opts ...ClientOption) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	clientOpts := &clientOptions{}
	for _, opt := range opts {
		opt(clientOpts)
	}

	if kubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig is required")
	}
	clientCfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	overrides := &clientcmd.ConfigOverrides{}
	if clientOpts.contextName != "" {
		overrides.CurrentContext = clientOpts.contextName
	}
	config, err = clientcmd.NewDefaultClientConfig(*clientCfg, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config from kubeconfig: %w", err)
	}
	if clientOpts.timeout != nil {
		config.Timeout = *clientOpts.timeout
	} else {
		config.Timeout = 30 * time.Second
	}
	if clientOpts.dial != nil {
		config.Dial = clientOpts.dial
		config.Proxy = func(*http.Request) (*url.URL, error) {
			return nil, nil
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create k8s clientset: %w", err)
	}
	return clientset, nil
}

type PodListItem struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	NodeName     string `json:"node_name"`
	PodIP        string `json:"pod_ip"`
	Age          string `json:"age"`
	Ready        string `json:"ready"`
	RestartCount int32  `json:"restart_count"`
}

type DeploymentListItem struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Ready     string        `json:"ready"`
	UpToDate  int32         `json:"up_to_date"`
	Available int32         `json:"available"`
	Age       string        `json:"age"`
	Pods      []PodListItem `json:"pods"`
}

type ServicePortItem struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	NodePort   int32  `json:"node_port"`
	Protocol   string `json:"protocol"`
}

type ServiceListItem struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	ClusterIP string            `json:"cluster_ip"`
	Ports     []ServicePortItem `json:"ports"`
	Age       string            `json:"age"`
}

type ConfigMapListItem struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Data      map[string]string `json:"data"`
	Age       string            `json:"age"`
}

type SecretListItem struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data"`
	Age       string            `json:"age"`
}

type ContainerDetail struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
}

type ConditionDetail struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type EventDetail struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	FirstTime string `json:"first_time"`
	LastTime  string `json:"last_time"`
	Count     int32  `json:"count"`
}

type PodDetail struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Status       string            `json:"status"`
	NodeName     string            `json:"node_name"`
	PodIP        string            `json:"pod_ip"`
	HostIP       string            `json:"host_ip"`
	CreationTime string            `json:"creation_time"`
	Age          string            `json:"age"`
	Ready        string            `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	QosClass     string            `json:"qos_class"`
	Containers   []ContainerDetail `json:"containers"`
	Conditions   []ConditionDetail `json:"conditions"`
	Events       []EventDetail     `json:"events"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	YAML         string            `json:"yaml"`
}

type NamespaceResources struct {
	Namespace       string `json:"namespace"`
	Pods            int    `json:"pods"`
	Deployments     int    `json:"deployments"`
	Services        int    `json:"services"`
	ConfigMaps      int    `json:"config_maps"`
	Secrets         int    `json:"secrets"`
	PVCs            int    `json:"pvcs"`
	ServiceAccounts int    `json:"service_accounts"`
}

func GetNamespaceResources(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) (*NamespaceResources, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	res := &NamespaceResources{Namespace: namespace}

	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	res.Pods = len(podList.Items)

	deployList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	res.Deployments = len(deployList.Items)

	svcList, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	res.Services = len(svcList.Items)

	cmList, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}
	res.ConfigMaps = len(cmList.Items)

	secretList, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	res.Secrets = len(secretList.Items)

	pvcList, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pvcs: %w", err)
	}
	res.PVCs = len(pvcList.Items)

	saList, err := clientset.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list serviceaccounts: %w", err)
	}
	res.ServiceAccounts = len(saList.Items)

	return res, nil
}

func getNodeRoles(node *corev1.Node) []string {
	roles := []string{}
	for label := range node.Labels {
		if label == "node-role.kubernetes.io/control-plane" || label == "node-role.kubernetes.io/master" {
			roles = append(roles, "control-plane")
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}
	return roles
}

func GetNamespacePods(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) ([]PodListItem, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	now := time.Now()
	result := make([]PodListItem, 0, len(podList.Items))
	for _, pod := range podList.Items {
		result = append(result, podListItem(pod, now))
	}
	return result, nil
}

func podListItem(pod corev1.Pod, now time.Time) PodListItem {
	readyContainers := int32(0)
	totalContainers := len(pod.Spec.Containers)
	status := string(pod.Status.Phase)
	restarts := int32(0)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyContainers++
		}
		restarts += cs.RestartCount
	}
	if pod.Status.Reason != "" {
		status = pod.Status.Reason
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			status = cs.State.Waiting.Reason
		}
	}
	return PodListItem{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		Status:       status,
		NodeName:     pod.Spec.NodeName,
		PodIP:        pod.Status.PodIP,
		Age:          fmtDuration(now.Sub(pod.CreationTimestamp.Time)),
		Ready:        fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		RestartCount: restarts,
	}
}

func GetNamespaceDeployments(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) ([]DeploymentListItem, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	deployList, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	now := time.Now()
	result := make([]DeploymentListItem, 0, len(deployList.Items))
	for _, deploy := range deployList.Items {
		selector, err := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("build deployment selector %s: %w", deploy.Name, err)
		}

		pods := make([]PodListItem, 0)
		for _, pod := range podList.Items {
			if selector.Matches(labels.Set(pod.Labels)) {
				pods = append(pods, podListItem(pod, now))
			}
		}

		desired := int32(0)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}
		result = append(result, DeploymentListItem{
			Name:      deploy.Name,
			Namespace: deploy.Namespace,
			Ready:     fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, desired),
			UpToDate:  deploy.Status.UpdatedReplicas,
			Available: deploy.Status.AvailableReplicas,
			Age:       fmtDuration(now.Sub(deploy.CreationTimestamp.Time)),
			Pods:      pods,
		})
	}
	return result, nil
}

func GetNamespaceServices(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) ([]ServiceListItem, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	svcList, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	now := time.Now()
	result := make([]ServiceListItem, 0, len(svcList.Items))
	for _, svc := range svcList.Items {
		ports := make([]ServicePortItem, 0, len(svc.Spec.Ports))
		for _, p := range svc.Spec.Ports {
			targetPort := p.TargetPort.String()
			if targetPort == "0" {
				targetPort = ""
			}
			ports = append(ports, ServicePortItem{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: targetPort,
				NodePort:   p.NodePort,
				Protocol:   string(p.Protocol),
			})
		}

		result = append(result, ServiceListItem{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			Ports:     ports,
			Age:       fmtDuration(now.Sub(svc.CreationTimestamp.Time)),
		})
	}
	return result, nil
}

func GetNamespaceConfigMaps(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) ([]ConfigMapListItem, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	cmList, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}

	now := time.Now()
	result := make([]ConfigMapListItem, 0, len(cmList.Items))
	for _, cm := range cmList.Items {
		result = append(result, ConfigMapListItem{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Data:      cm.Data,
			Age:       fmtDuration(now.Sub(cm.CreationTimestamp.Time)),
		})
	}
	return result, nil
}

func GetNamespaceSecrets(ctx context.Context, kubeconfig, namespace string, opts ...ClientOption) ([]SecretListItem, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	secretList, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	now := time.Now()
	result := make([]SecretListItem, 0, len(secretList.Items))
	for _, s := range secretList.Items {
		data := make(map[string]string, len(s.Data))
		for k, v := range s.Data {
			data[k] = base64.StdEncoding.EncodeToString(v)
		}
		result = append(result, SecretListItem{
			Name:      s.Name,
			Namespace: s.Namespace,
			Type:      string(s.Type),
			Data:      data,
			Age:       fmtDuration(now.Sub(s.CreationTimestamp.Time)),
		})
	}
	return result, nil
}

func GetPodDetail(ctx context.Context, kubeconfig, namespace, podName string, opts ...ClientOption) (*PodDetail, error) {
	clientset, err := buildClient(kubeconfig, opts...)
	if err != nil {
		return nil, err
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s/%s: %w", namespace, podName, err)
	}

	now := time.Now()

	readyContainers := int32(0)
	totalContainers := len(pod.Spec.Containers)
	restarts := int32(0)
	statusByName := make(map[string]*corev1.ContainerStatus, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		cs := &pod.Status.ContainerStatuses[i]
		statusByName[cs.Name] = cs
	}
	containers := make([]ContainerDetail, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		state := "Unknown"
		ready := false
		cr := int32(0)
		if cs, ok := statusByName[c.Name]; ok {
			ready = cs.Ready
			cr = cs.RestartCount
			restarts += cr
			if ready {
				readyContainers++
			}
			if cs.State.Running != nil {
				state = "Running"
			} else if cs.State.Waiting != nil {
				state = "Waiting: " + cs.State.Waiting.Reason
			} else if cs.State.Terminated != nil {
				state = "Terminated: " + cs.State.Terminated.Reason
			}
		}
		containers = append(containers, ContainerDetail{
			Name:         c.Name,
			Image:        c.Image,
			State:        state,
			Ready:        ready,
			RestartCount: cr,
		})
	}

	status := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		status = pod.Status.Reason
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			status = cs.State.Waiting.Reason
		}
	}

	conditions := make([]ConditionDetail, 0, len(pod.Status.Conditions))
	for _, c := range pod.Status.Conditions {
		conditions = append(conditions, ConditionDetail{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	events, _ := getPodEvents(ctx, clientset, namespace, podName)

	jsonBytes, err := json.Marshal(&pod)
	var yamlData []byte
	if err == nil {
		yamlData, err = yaml.JSONToYAML(jsonBytes)
	}
	if err != nil {
		yamlData = []byte(fmt.Sprintf("error marshaling YAML: %v", err))
	}

	labels := make(map[string]string)
	for k, v := range pod.Labels {
		labels[k] = v
	}
	annotations := make(map[string]string)
	for k, v := range pod.Annotations {
		annotations[k] = v
	}

	return &PodDetail{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		Status:       status,
		NodeName:     pod.Spec.NodeName,
		PodIP:        pod.Status.PodIP,
		HostIP:       pod.Status.HostIP,
		CreationTime: pod.CreationTimestamp.Format("2006-01-02 15:04:05"),
		Age:          fmtDuration(now.Sub(pod.CreationTimestamp.Time)),
		Ready:        fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		RestartCount: restarts,
		QosClass:     string(pod.Status.QOSClass),
		Containers:   containers,
		Conditions:   conditions,
		Events:       events,
		Labels:       labels,
		Annotations:  annotations,
		YAML:         string(yamlData),
	}, nil
}

func getPodEvents(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) ([]EventDetail, error) {
	eventsList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + podName,
	})
	if err != nil {
		return nil, err
	}

	result := make([]EventDetail, 0, len(eventsList.Items))
	for _, e := range eventsList.Items {
		result = append(result, EventDetail{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			FirstTime: e.FirstTimestamp.Format("2006-01-02 15:04:05"),
			LastTime:  e.LastTimestamp.Format("2006-01-02 15:04:05"),
			Count:     e.Count,
		})
	}
	if result == nil {
		result = []EventDetail{}
	}
	return result, nil
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, h)
}

func StreamPodLogs(ctx context.Context, kubeconfig, namespace, podName, container string, tailLines int64, opts ...ClientOption) (io.ReadCloser, error) {
	streamOpts := append(append([]ClientOption{}, opts...), WithTimeout(0))
	clientset, err := buildClient(kubeconfig, streamOpts...)
	if err != nil {
		return nil, err
	}

	logOpts := &corev1.PodLogOptions{
		Container: container,
		Follow:    true,
		TailLines: &tailLines,
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("stream pod logs: %w", err)
	}
	return stream, nil
}
