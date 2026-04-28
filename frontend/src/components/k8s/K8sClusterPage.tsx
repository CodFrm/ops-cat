import { useState, useEffect, useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Loader2,
  Server,
  Box,
  Layers,
  RefreshCw,
  Circle,
  Grid3X3,
  Container,
  FileText,
  Key,
  AlertCircle,
  ChevronRight,
  ChevronDown,
  Search,
} from "lucide-react";
import type { asset_entity } from "../../../wailsjs/go/models";
import {
  GetK8sClusterInfo,
  GetK8sNamespaceResources,
  GetK8sNamespacePods,
  GetK8sNamespaceDeployments,
  GetK8sNamespaceServices,
  GetK8sNamespaceConfigMaps,
  GetK8sNamespaceSecrets,
  GetK8sPodDetail,
  StartK8sPodLogs,
  StopK8sPodLogs,
} from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { useResizeHandle } from "@opskat/ui";
import { InfoItem } from "@/components/asset/detail/InfoItem";
import { K8sSectionCard } from "./K8sSectionCard";
import { K8sResourceHeader } from "./K8sResourceHeader";
import { K8sMetadataGrid } from "./K8sMetadataGrid";
import { K8sTableSection } from "./K8sTableSection";
import { K8sConditionList } from "./K8sConditionList";
import { K8sTagList } from "./K8sTagList";
import { K8sCodeBlock } from "./K8sCodeBlock";
import { K8sLogsPanel } from "./K8sLogsPanel";
import { getK8sStatusColor, getContainerStateColor, statusVariantToClass } from "./utils";

interface NodeInfo {
  name: string;
  status: string;
  roles: string[];
  version: string;
  cpu: string;
  memory: string;
  os: string;
  arch: string;
}

interface NamespaceInfo {
  name: string;
  status: string;
}

interface NamespaceResourcesData {
  namespace: string;
  pods: number;
  deployments: number;
  services: number;
  config_maps: number;
  secrets: number;
  pvcs: number;
  service_accounts: number;
}

interface ClusterInfo {
  version: string;
  platform: string;
  nodes: NodeInfo[];
  namespaces: NamespaceInfo[];
}

type InnerTabId =
  | "overview"
  | `node:${string}`
  | `ns:${string}`
  | `ns-res:${string}:${string}`
  | `pod:${string}:${string}`
  | `svc:${string}:${string}`
  | `cm:${string}:${string}`
  | `secret:${string}:${string}`;

interface InnerTab {
  id: InnerTabId;
  label: string;
}

interface ResourceTypeDef {
  key: keyof NamespaceResourcesData;
  labelKey: string;
  icon: React.FC<{ className?: string; style?: React.CSSProperties }>;
}

interface PodListItem {
  name: string;
  namespace: string;
  status: string;
  node_name: string;
  pod_ip: string;
  age: string;
  ready: string;
  restart_count: number;
}

interface DeploymentListItem {
  name: string;
  namespace: string;
  ready: string;
  up_to_date: number;
  available: number;
  age: string;
  pods: PodListItem[];
}

interface ServicePortItem {
  name: string;
  port: number;
  target_port: string;
  node_port: number;
  protocol: string;
}

interface ServiceListItem {
  name: string;
  namespace: string;
  type: string;
  cluster_ip: string;
  ports: ServicePortItem[];
  age: string;
}

interface ConfigMapListItem {
  name: string;
  namespace: string;
  data: Record<string, string>;
  age: string;
}

interface SecretListItem {
  name: string;
  namespace: string;
  type: string;
  data: Record<string, string>;
  age: string;
}

interface ContainerDetail {
  name: string;
  image: string;
  state: string;
  ready: boolean;
  restart_count: number;
}

interface ConditionDetail {
  type: string;
  status: string;
  reason: string;
  message: string;
}

interface EventDetail {
  type: string;
  reason: string;
  message: string;
  first_time: string;
  last_time: string;
  count: number;
}

interface PodDetail {
  name: string;
  namespace: string;
  status: string;
  node_name: string;
  pod_ip: string;
  host_ip: string;
  creation_time: string;
  age: string;
  ready: string;
  restart_count: number;
  qos_class: string;
  containers: ContainerDetail[];
  conditions: ConditionDetail[];
  events: EventDetail[];
  labels: Record<string, string>;
  annotations: Record<string, string>;
  yaml: string;
}

const RESOURCE_TYPES: ResourceTypeDef[] = [
  { key: "pods", labelKey: "asset.k8sPods", icon: Circle },
  { key: "deployments", labelKey: "asset.k8sDeployments", icon: Grid3X3 },
  { key: "services", labelKey: "asset.k8sServices", icon: Container },
  { key: "config_maps", labelKey: "asset.k8sConfigMaps", icon: FileText },
  { key: "secrets", labelKey: "asset.k8sSecrets", icon: Key },
];

interface Props {
  asset: asset_entity.Asset;
}

export function K8sClusterPage({ asset }: Props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [info, setInfo] = useState<ClusterInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [innerTabs, setInnerTabs] = useState<InnerTab[]>([{ id: "overview", label: t("asset.k8sClusterOverview") }]);
  const [activeTabId, setActiveTabId] = useState<InnerTabId>("overview");
  const [expandedNodes, setExpandedNodes] = useState(false);
  const [expandedNamespaces, setExpandedNamespaces] = useState<Set<string>>(new Set());
  const [expandedPods, setExpandedPods] = useState<Set<string>>(new Set());
  const [expandedDeployments, setExpandedDeployments] = useState<Set<string>>(new Set());
  const [expandedServices, setExpandedServices] = useState<Set<string>>(new Set());
  const [expandedConfigMaps, setExpandedConfigMaps] = useState<Set<string>>(new Set());
  const [expandedSecrets, setExpandedSecrets] = useState<Set<string>>(new Set());
  const [expandedDeploymentItems, setExpandedDeploymentItems] = useState<Set<string>>(new Set());
  const [namespaceResourceSearch, setNamespaceResourceSearch] = useState<Record<string, string>>({});
  const [namespaceResources, setNamespaceResources] = useState<Record<string, NamespaceResourcesData>>({});
  const [loadingNamespaces, setLoadingNamespaces] = useState<Set<string>>(new Set());
  const [namespaceErrors, setNamespaceErrors] = useState<Record<string, string>>({});
  const [namespacePodList, setNamespacePodList] = useState<Record<string, PodListItem[]>>({});
  const [loadingPods, setLoadingPods] = useState<Set<string>>(new Set());
  const [podErrors, setPodErrors] = useState<Record<string, string>>({});
  const [namespaceDeploymentList, setNamespaceDeploymentList] = useState<Record<string, DeploymentListItem[]>>({});
  const [loadingDeployments, setLoadingDeployments] = useState<Set<string>>(new Set());
  const [deploymentErrors, setDeploymentErrors] = useState<Record<string, string>>({});
  const [namespaceServiceList, setNamespaceServiceList] = useState<Record<string, ServiceListItem[]>>({});
  const [loadingServices, setLoadingServices] = useState<Set<string>>(new Set());
  const [serviceErrors, setServiceErrors] = useState<Record<string, string>>({});
  const [namespaceConfigMapList, setNamespaceConfigMapList] = useState<Record<string, ConfigMapListItem[]>>({});
  const [loadingConfigMaps, setLoadingConfigMaps] = useState<Set<string>>(new Set());
  const [configMapErrors, setConfigMapErrors] = useState<Record<string, string>>({});
  const [namespaceSecretList, setNamespaceSecretList] = useState<Record<string, SecretListItem[]>>({});
  const [loadingSecrets, setLoadingSecrets] = useState<Set<string>>(new Set());
  const [secretErrors, setSecretErrors] = useState<Record<string, string>>({});
  const [podDetails, setPodDetails] = useState<Record<string, PodDetail>>({});
  const [loadingPodDetails, setLoadingPodDetails] = useState<Set<string>>(new Set());
  const [podDetailErrors, setPodDetailErrors] = useState<Record<string, string>>({});
  const [logStreamID, setLogStreamID] = useState<string | null>(null);
  const [logLines, setLogLines] = useState<string[]>([]);
  const [logContainer, setLogContainer] = useState("");
  const [logTailLines, setLogTailLines] = useState(200);
  const [logError, setLogError] = useState<string | null>(null);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const logStreamIDRef = useRef<string | null>(null);
  const {
    size: sidebarWidth,
    isResizing: sidebarResizing,
    handleMouseDown: handleSidebarResize,
  } = useResizeHandle({
    defaultSize: 208,
    minSize: 160,
    maxSize: 420,
    storageKey: "k8s_sidebar_width",
    targetRef: sidebarRef,
  });

  const loadInfo = () => {
    setLoading(true);
    setError(null);
    GetK8sClusterInfo(asset.ID)
      .then((result: string) => {
        const data = JSON.parse(result) as ClusterInfo;
        setInfo(data);
        setInnerTabs([{ id: "overview", label: t("asset.k8sClusterOverview") }]);
        setActiveTabId("overview");
        setExpandedNamespaces(new Set());
        setExpandedPods(new Set());
        setExpandedDeployments(new Set());
        setExpandedServices(new Set());
        setExpandedConfigMaps(new Set());
        setExpandedSecrets(new Set());
        setExpandedDeploymentItems(new Set());
        setNamespaceResourceSearch({});
        setNamespaceResources({});
        setLoadingNamespaces(new Set());
        setNamespaceErrors({});
        setNamespacePodList({});
        setLoadingPods(new Set());
        setPodErrors({});
        setNamespaceDeploymentList({});
        setLoadingDeployments(new Set());
        setDeploymentErrors({});
        setNamespaceServiceList({});
        setLoadingServices(new Set());
        setServiceErrors({});
        setNamespaceConfigMapList({});
        setLoadingConfigMaps(new Set());
        setConfigMapErrors({});
        setNamespaceSecretList({});
        setLoadingSecrets(new Set());
        setSecretErrors({});
        setPodDetails({});
        setLoadingPodDetails(new Set());
        setPodDetailErrors({});
      })
      .catch((e: unknown) => {
        setError(String(e));
      })
      .finally(() => {
        setLoading(false);
      });
  };

  const loadNamespaceResources = useCallback(
    (ns: string) => {
      if (namespaceResources[ns] || loadingNamespaces.has(ns)) return;

      setLoadingNamespaces((prev) => new Set(prev).add(ns));
      GetK8sNamespaceResources(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as NamespaceResourcesData;
          setNamespaceResources((prev) => ({ ...prev, [ns]: data }));
          setNamespaceErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setNamespaceErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingNamespaces((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespaceResources, loadingNamespaces]
  );

  const toggleNamespace = (ns: string) => {
    setExpandedNamespaces((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadNamespaceResources(ns);
      }
      return next;
    });
  };

  const loadPods = useCallback(
    (ns: string) => {
      if (namespacePodList[ns] || loadingPods.has(ns)) return;

      setLoadingPods((prev) => new Set(prev).add(ns));
      GetK8sNamespacePods(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as PodListItem[];
          setNamespacePodList((prev) => ({ ...prev, [ns]: data }));
          setPodErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setPodErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingPods((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespacePodList, loadingPods]
  );

  const togglePods = (ns: string) => {
    setExpandedPods((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadPods(ns);
      }
      return next;
    });
  };

  const loadDeployments = useCallback(
    (ns: string) => {
      if (namespaceDeploymentList[ns] || loadingDeployments.has(ns)) return;

      setLoadingDeployments((prev) => new Set(prev).add(ns));
      GetK8sNamespaceDeployments(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as DeploymentListItem[];
          setNamespaceDeploymentList((prev) => ({ ...prev, [ns]: data }));
          setDeploymentErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setDeploymentErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingDeployments((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespaceDeploymentList, loadingDeployments]
  );

  const loadServices = useCallback(
    (ns: string) => {
      if (namespaceServiceList[ns] || loadingServices.has(ns)) return;

      setLoadingServices((prev) => new Set(prev).add(ns));
      GetK8sNamespaceServices(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as ServiceListItem[];
          setNamespaceServiceList((prev) => ({ ...prev, [ns]: data }));
          setServiceErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setServiceErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingServices((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespaceServiceList, loadingServices]
  );

  const loadConfigMaps = useCallback(
    (ns: string) => {
      if (namespaceConfigMapList[ns] || loadingConfigMaps.has(ns)) return;

      setLoadingConfigMaps((prev) => new Set(prev).add(ns));
      GetK8sNamespaceConfigMaps(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as ConfigMapListItem[];
          setNamespaceConfigMapList((prev) => ({ ...prev, [ns]: data }));
          setConfigMapErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setConfigMapErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingConfigMaps((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespaceConfigMapList, loadingConfigMaps]
  );

  const loadSecrets = useCallback(
    (ns: string) => {
      if (namespaceSecretList[ns] || loadingSecrets.has(ns)) return;

      setLoadingSecrets((prev) => new Set(prev).add(ns));
      GetK8sNamespaceSecrets(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as SecretListItem[];
          setNamespaceSecretList((prev) => ({ ...prev, [ns]: data }));
          setSecretErrors((prev) => {
            const next = { ...prev };
            delete next[ns];
            return next;
          });
        })
        .catch((e: unknown) => {
          setSecretErrors((prev) => ({ ...prev, [ns]: String(e) }));
        })
        .finally(() => {
          setLoadingSecrets((prev) => {
            const next = new Set(prev);
            next.delete(ns);
            return next;
          });
        });
    },
    [asset.ID, namespaceSecretList, loadingSecrets]
  );

  const toggleDeployments = (ns: string) => {
    setExpandedDeployments((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadDeployments(ns);
      }
      return next;
    });
  };

  const toggleServices = (ns: string) => {
    setExpandedServices((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadServices(ns);
      }
      return next;
    });
  };

  const toggleConfigMaps = (ns: string) => {
    setExpandedConfigMaps((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadConfigMaps(ns);
      }
      return next;
    });
  };

  const toggleSecrets = (ns: string) => {
    setExpandedSecrets((prev) => {
      const next = new Set(prev);
      if (next.has(ns)) {
        next.delete(ns);
      } else {
        next.add(ns);
        loadSecrets(ns);
      }
      return next;
    });
  };

  const toggleDeploymentItem = (ns: string, deploymentName: string) => {
    const key = `${ns}/${deploymentName}`;
    setExpandedDeploymentItems((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const loadPodDetail = useCallback(
    (ns: string, podName: string) => {
      const key = `${ns}/${podName}`;
      if (podDetails[key] || loadingPodDetails.has(key)) return;

      setLoadingPodDetails((prev) => new Set(prev).add(key));
      GetK8sPodDetail(asset.ID, ns, podName)
        .then((result: string) => {
          const data = JSON.parse(result) as PodDetail;
          setPodDetails((prev) => ({ ...prev, [key]: data }));
          setPodDetailErrors((prev) => {
            const next = { ...prev };
            delete next[key];
            return next;
          });
        })
        .catch((e: unknown) => {
          setPodDetailErrors((prev) => ({ ...prev, [key]: String(e) }));
        })
        .finally(() => {
          setLoadingPodDetails((prev) => {
            const next = new Set(prev);
            next.delete(key);
            return next;
          });
        });
    },
    [asset.ID, podDetails, loadingPodDetails]
  );

  const stopLogStream = useCallback(() => {
    const sid = logStreamIDRef.current;
    if (sid) {
      StopK8sPodLogs(sid);
    }
    logStreamIDRef.current = null;
    setLogStreamID(null);
  }, []);

  const startLogStream = useCallback(
    (ns: string, podName: string, container: string, tailLines: number) => {
      stopLogStream();
      setLogLines([]);
      setLogError(null);
      setLogContainer(container);

      StartK8sPodLogs(asset.ID, ns, podName, container, tailLines)
        .then((streamID: string) => {
          logStreamIDRef.current = streamID;
          setLogStreamID(streamID);
          const dataEvent = "k8s:log:" + streamID;
          const errEvent = "k8s:logerr:" + streamID;
          const endEvent = "k8s:logend:" + streamID;

          EventsOn(dataEvent, (data: string) => {
            const decoded = atob(data);
            setLogLines((prev) => [...prev, decoded]);
          });

          EventsOn(errEvent, (err: string) => {
            setLogError(err);
          });

          EventsOn(endEvent, () => {
            setLogStreamID(null);
            EventsOff(dataEvent);
            EventsOff(errEvent);
            EventsOff(endEvent);
          });
        })
        .catch((e: unknown) => {
          setLogError(String(e));
        });
    },
    [asset.ID, stopLogStream]
  );

  useEffect(() => {
    return () => {
      stopLogStream();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logLines]);

  useEffect(() => {
    loadInfo();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [asset.ID]);

  const activeNs =
    info && activeTabId.startsWith("ns:") ? info.namespaces.find((n) => n.name === activeTabId.slice(3)) : null;

  useEffect(() => {
    if (activeNs && !namespaceResources[activeNs.name] && !loadingNamespaces.has(activeNs.name)) {
      loadNamespaceResources(activeNs.name);
    }
  }, [activeNs, namespaceResources, loadingNamespaces, loadNamespaceResources]);

  const openTab = (id: InnerTabId, label: string) => {
    if (id === "overview") {
      setActiveTabId("overview");
      return;
    }
    if (!innerTabs.some((t) => t.id === id)) {
      setInnerTabs([...innerTabs, { id, label }]);
    }
    setActiveTabId(id);
    if (id.startsWith("pod:")) {
      const parts = id.split(":");
      const ns = parts[1];
      const podName = parts.slice(2).join(":");
      loadPodDetail(ns, podName);
    }
  };

  const closeTab = (id: InnerTabId) => {
    const idx = innerTabs.findIndex((t) => t.id === id);
    const next = innerTabs.filter((t) => t.id !== id);
    setInnerTabs(next);
    if (activeTabId === id) {
      const neighbor = innerTabs[idx + 1] || innerTabs[idx - 1];
      setActiveTabId(neighbor?.id || "overview");
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 p-8">
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive max-w-md text-center">
          {error}
        </div>
        <button
          onClick={loadInfo}
          className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm hover:bg-muted"
        >
          <RefreshCw className="h-3.5 w-3.5" />
          {t("action.retry")}
        </button>
      </div>
    );
  }

  if (!info) return null;

  const activeNode = activeTabId.startsWith("node:") ? info.nodes.find((n) => n.name === activeTabId.slice(5)) : null;
  const podMatchesSearch = (pod: PodListItem, query: string) => {
    const normalized = query.toLowerCase();
    return [pod.name, pod.status, pod.node_name, pod.pod_ip, pod.ready]
      .filter(Boolean)
      .some((value) => value.toLowerCase().includes(normalized));
  };
  const deploymentMatchesSearch = (deployment: DeploymentListItem, query: string) => {
    const normalized = query.toLowerCase();
    return (
      [deployment.name, deployment.ready, deployment.age].some((value) => value.toLowerCase().includes(normalized)) ||
      deployment.pods.some((pod) => podMatchesSearch(pod, normalized))
    );
  };

  return (
    <div className="flex h-full w-full">
      <div
        ref={sidebarRef}
        className="shrink-0 border-r border-border bg-sidebar h-full overflow-y-auto"
        style={{ width: sidebarWidth }}
      >
        <div className="p-3 border-b border-border">
          <h2 className="text-sm font-semibold truncate">{asset.Name}</h2>
          <p className="text-xs text-muted-foreground mt-0.5">v{info.version}</p>
        </div>

        <div className="p-2">
          <button
            onClick={loadInfo}
            className="flex items-center gap-1.5 w-full rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:bg-muted/50 mb-1"
          >
            <RefreshCw className="h-3 w-3" />
            {t("action.refresh")}
          </button>

          <div
            className={`flex items-center gap-1.5 px-2 py-1.5 rounded-md text-xs cursor-pointer mb-0.5 ${
              activeTabId === "overview" ? "bg-muted font-medium" : "hover:bg-muted/50"
            }`}
            onClick={() => setActiveTabId("overview")}
          >
            <Server className="h-3.5 w-3.5" />
            {t("asset.k8sClusterOverview")}
          </div>

          <div
            className="flex items-center gap-1.5 px-2 py-1.5 rounded-md text-xs cursor-pointer hover:bg-muted/50"
            onClick={() => setExpandedNodes(!expandedNodes)}
          >
            <span className="text-[10px] w-3">{expandedNodes ? "\u25BC" : "\u25B6"}</span>
            <Box className="h-3.5 w-3.5" />
            {t("asset.k8sNodes")}
            <span className="ml-auto text-[10px] text-muted-foreground">{info.nodes.length}</span>
          </div>
          {expandedNodes &&
            info.nodes.map((node) => (
              <div
                key={node.name}
                className={`flex items-center gap-1.5 pl-8 pr-2 py-1.5 rounded-md text-xs cursor-pointer ml-1 ${
                  activeTabId === `node:${node.name}` ? "bg-muted font-medium" : "hover:bg-muted/50"
                }`}
                onClick={() => openTab(`node:${node.name}`, node.name)}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                    node.status === "True" ? "bg-green-500" : "bg-red-500"
                  }`}
                />
                <span className="truncate">{node.name}</span>
              </div>
            ))}

          <div className="flex items-center gap-1.5 px-2 py-1.5 rounded-md text-xs text-muted-foreground/70 mt-1">
            <Layers className="h-3.5 w-3.5" />
            {t("asset.k8sNamespaces")}
            <span className="ml-auto text-[10px]">{info.namespaces.length}</span>
          </div>
          {info.namespaces.map((ns) => (
            <div key={ns.name}>
              <div
                className="flex items-center gap-1.5 pl-6 pr-2 py-1.5 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                onClick={() => toggleNamespace(ns.name)}
              >
                <span className="text-[10px] w-3 translate-x-[-2px]">
                  {expandedNamespaces.has(ns.name) ? "\u25BC" : "\u25B6"}
                </span>
                <span className="truncate">{ns.name}</span>
              </div>
              {expandedNamespaces.has(ns.name) && (
                <div className="ml-3">
                  {loadingNamespaces.has(ns.name) && (
                    <div className="flex items-center gap-1.5 pl-8 pr-2 py-1 text-xs text-muted-foreground">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      {t("asset.k8sLoadingNamespace")}
                    </div>
                  )}
                  {namespaceErrors[ns.name] && (
                    <div
                      className="flex items-start gap-1 pl-8 pr-2 py-1 text-xs text-destructive cursor-pointer"
                      title={namespaceErrors[ns.name]}
                      onClick={() => {
                        const next = { ...namespaceErrors };
                        delete next[ns.name];
                        setNamespaceErrors(next);
                        loadNamespaceResources(ns.name);
                      }}
                    >
                      <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                      <span>{t("asset.k8sNamespaceResourceError")}</span>
                    </div>
                  )}
                  {namespaceResources[ns.name] &&
                    (() => {
                      const query = (namespaceResourceSearch[ns.name] || "").trim();
                      const normalizedQuery = query.toLowerCase();
                      const visibleResourceTypes = RESOURCE_TYPES.filter((rt) => {
                        if (!normalizedQuery) return true;
                        const resourceLabel = t(rt.labelKey).toLowerCase();
                        if (resourceLabel.includes(normalizedQuery) || rt.key.includes(normalizedQuery)) return true;
                        if (rt.key === "deployments") {
                          return namespaceDeploymentList[ns.name]?.some((deployment) =>
                            deploymentMatchesSearch(deployment, normalizedQuery)
                          );
                        }
                        if (rt.key !== "pods") return false;
                        return namespacePodList[ns.name]?.some((pod) => podMatchesSearch(pod, normalizedQuery));
                      });

                      return (
                        <>
                          <div className="relative my-1 ml-7 mr-2">
                            <Search className="absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-muted-foreground" />
                            <input
                              value={namespaceResourceSearch[ns.name] || ""}
                              onChange={(e) =>
                                setNamespaceResourceSearch((prev) => ({ ...prev, [ns.name]: e.target.value }))
                              }
                              placeholder={t("asset.search")}
                              className="h-7 w-full rounded-md border bg-background pl-7 pr-2 text-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[2px] focus-visible:ring-ring/40"
                            />
                          </div>
                          {visibleResourceTypes.length === 0 && (
                            <div className="flex items-center gap-1.5 pl-8 pr-2 py-1 text-xs text-muted-foreground">
                              {t("asset.k8sNoResourceMatches")}
                            </div>
                          )}
                          {visibleResourceTypes.map((rt) => {
                            const count = namespaceResources[ns.name][rt.key] as number;
                            const isPods = rt.key === "pods";
                            const isDeployments = rt.key === "deployments";
                            const isServices = rt.key === "services";
                            const isConfigMaps = rt.key === "config_maps";
                            const isSecrets = rt.key === "secrets";
                            const podsExpanded = expandedPods.has(ns.name);
                            const deploymentsExpanded = expandedDeployments.has(ns.name);
                            const servicesExpanded = expandedServices.has(ns.name);
                            const configMapsExpanded = expandedConfigMaps.has(ns.name);
                            const secretsExpanded = expandedSecrets.has(ns.name);
                            if (isDeployments) {
                              const deployments = namespaceDeploymentList[ns.name];
                              const visibleDeployments = normalizedQuery
                                ? deployments?.filter((deployment) =>
                                    deploymentMatchesSearch(deployment, normalizedQuery)
                                  )
                                : deployments;
                              const displayCount =
                                normalizedQuery && deployments ? visibleDeployments?.length || 0 : count;
                              return (
                                <div key={rt.key}>
                                  <div
                                    className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                    onClick={() => toggleDeployments(ns.name)}
                                  >
                                    {deploymentsExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    )}
                                    <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                    <span className="truncate">{t(rt.labelKey)}</span>
                                    <span className="ml-auto text-[10px] text-muted-foreground">{displayCount}</span>
                                  </div>
                                  {deploymentsExpanded && (
                                    <div className="ml-3">
                                      {loadingDeployments.has(ns.name) && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("asset.k8sLoadingDeployments")}
                                        </div>
                                      )}
                                      {deploymentErrors[ns.name] && (
                                        <div
                                          className="flex items-start gap-1 pl-12 pr-2 py-1 text-xs text-destructive cursor-pointer"
                                          title={deploymentErrors[ns.name]}
                                          onClick={() => {
                                            const next = { ...deploymentErrors };
                                            delete next[ns.name];
                                            setDeploymentErrors(next);
                                            loadDeployments(ns.name);
                                          }}
                                        >
                                          <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                          <span>{t("asset.k8sNamespaceResourceError")}</span>
                                        </div>
                                      )}
                                      {visibleDeployments?.length === 0 && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          {t("asset.k8sNoDeployments")}
                                        </div>
                                      )}
                                      {visibleDeployments?.map((deployment) => {
                                        const deploymentKey = `${ns.name}/${deployment.name}`;
                                        const deploymentExpanded = expandedDeploymentItems.has(deploymentKey);
                                        const visiblePods = normalizedQuery
                                          ? deployment.pods.filter((pod) => podMatchesSearch(pod, normalizedQuery))
                                          : deployment.pods;
                                        return (
                                          <div key={deployment.name}>
                                            <div
                                              className="flex items-center gap-1.5 pl-12 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                              onClick={() => toggleDeploymentItem(ns.name, deployment.name)}
                                            >
                                              {deploymentExpanded ? (
                                                <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                              ) : (
                                                <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                              )}
                                              <span className="w-1.5 h-1.5 rounded-full bg-blue-500 shrink-0" />
                                              <span className="truncate">{deployment.name}</span>
                                              <span className="ml-auto text-[10px] text-muted-foreground">
                                                {deployment.ready}
                                              </span>
                                            </div>
                                            {deploymentExpanded && (
                                              <>
                                                {visiblePods.length === 0 && (
                                                  <div className="flex items-center gap-1.5 pl-20 pr-2 py-1 text-xs text-muted-foreground">
                                                    {t("asset.k8sNoPods")}
                                                  </div>
                                                )}
                                                {visiblePods.map((pod) => (
                                                  <div
                                                    key={pod.name}
                                                    className={`flex items-center gap-1.5 pl-20 pr-2 py-1 rounded-md text-xs cursor-pointer ml-1 ${
                                                      activeTabId === `pod:${ns.name}:${pod.name}`
                                                        ? "bg-muted font-medium"
                                                        : "hover:bg-muted/50"
                                                    }`}
                                                    onClick={() => openTab(`pod:${ns.name}:${pod.name}`, pod.name)}
                                                  >
                                                    <span
                                                      className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                                                        pod.status === "Running"
                                                          ? "bg-green-500"
                                                          : pod.status === "Pending"
                                                            ? "bg-yellow-500"
                                                            : "bg-red-500"
                                                      }`}
                                                    />
                                                    <span className="truncate">{pod.name}</span>
                                                  </div>
                                                ))}
                                              </>
                                            )}
                                          </div>
                                        );
                                      })}
                                    </div>
                                  )}
                                </div>
                              );
                            }
                            if (isPods) {
                              const pods = namespacePodList[ns.name];
                              const visiblePods = normalizedQuery
                                ? pods?.filter((pod) => podMatchesSearch(pod, normalizedQuery))
                                : pods;
                              const displayCount = normalizedQuery && pods ? visiblePods?.length || 0 : count;
                              return (
                                <div key={rt.key}>
                                  <div
                                    className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                    onClick={() => togglePods(ns.name)}
                                  >
                                    {podsExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    )}
                                    <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                    <span className="truncate">{t(rt.labelKey)}</span>
                                    <span className="ml-auto text-[10px] text-muted-foreground">{displayCount}</span>
                                  </div>
                                  {podsExpanded && (
                                    <div className="ml-3">
                                      {loadingPods.has(ns.name) && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("asset.k8sLoadingPods")}
                                        </div>
                                      )}
                                      {podErrors[ns.name] && (
                                        <div
                                          className="flex items-start gap-1 pl-12 pr-2 py-1 text-xs text-destructive cursor-pointer"
                                          title={podErrors[ns.name]}
                                          onClick={() => {
                                            const next = { ...podErrors };
                                            delete next[ns.name];
                                            setPodErrors(next);
                                            loadPods(ns.name);
                                          }}
                                        >
                                          <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                          <span>{t("asset.k8sNamespaceResourceError")}</span>
                                        </div>
                                      )}
                                      {visiblePods?.length === 0 && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          {t("asset.k8sNoPods")}
                                        </div>
                                      )}
                                      {visiblePods?.map((pod) => (
                                        <div
                                          key={pod.name}
                                          className={`flex items-center gap-1.5 pl-12 pr-2 py-1 rounded-md text-xs cursor-pointer ml-1 ${
                                            activeTabId === `pod:${ns.name}:${pod.name}`
                                              ? "bg-muted font-medium"
                                              : "hover:bg-muted/50"
                                          }`}
                                          onClick={() => openTab(`pod:${ns.name}:${pod.name}`, pod.name)}
                                        >
                                          <span
                                            className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                                              pod.status === "Running"
                                                ? "bg-green-500"
                                                : pod.status === "Pending"
                                                  ? "bg-yellow-500"
                                                  : "bg-red-500"
                                            }`}
                                          />
                                          <span className="truncate">{pod.name}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              );
                            }
                            if (isServices) {
                              const services = namespaceServiceList[ns.name];
                              const visibleServices = normalizedQuery
                                ? services?.filter((svc) => {
                                    const q = normalizedQuery;
                                    return [svc.name, svc.type, svc.cluster_ip]
                                      .filter(Boolean)
                                      .some((value) => value.toLowerCase().includes(q));
                                  })
                                : services;
                              const displayCount = normalizedQuery && services ? visibleServices?.length || 0 : count;
                              return (
                                <div key={rt.key}>
                                  <div
                                    className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                    onClick={() => toggleServices(ns.name)}
                                  >
                                    {servicesExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    )}
                                    <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                    <span className="truncate">{t(rt.labelKey)}</span>
                                    <span className="ml-auto text-[10px] text-muted-foreground">{displayCount}</span>
                                  </div>
                                  {servicesExpanded && (
                                    <div className="ml-3">
                                      {loadingServices.has(ns.name) && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("asset.k8sLoadingServices")}
                                        </div>
                                      )}
                                      {serviceErrors[ns.name] && (
                                        <div
                                          className="flex items-start gap-1 pl-12 pr-2 py-1 text-xs text-destructive cursor-pointer"
                                          title={serviceErrors[ns.name]}
                                          onClick={() => {
                                            const next = { ...serviceErrors };
                                            delete next[ns.name];
                                            setServiceErrors(next);
                                            loadServices(ns.name);
                                          }}
                                        >
                                          <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                          <span>{t("asset.k8sNamespaceResourceError")}</span>
                                        </div>
                                      )}
                                      {visibleServices?.length === 0 && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          {t("asset.k8sNoServices")}
                                        </div>
                                      )}
                                      {visibleServices?.map((svc) => (
                                        <div
                                          key={svc.name}
                                          className={`flex items-center gap-1.5 pl-12 pr-2 py-1 rounded-md text-xs cursor-pointer ml-1 ${
                                            activeTabId === `svc:${ns.name}:${svc.name}`
                                              ? "bg-muted font-medium"
                                              : "hover:bg-muted/50"
                                          }`}
                                          onClick={() => openTab(`svc:${ns.name}:${svc.name}`, svc.name)}
                                        >
                                          <Container className="h-3 w-3 shrink-0 text-muted-foreground" />
                                          <span className="truncate">{svc.name}</span>
                                          <span className="ml-auto text-[10px] text-muted-foreground">{svc.type}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              );
                            }
                            if (isConfigMaps) {
                              const configmaps = namespaceConfigMapList[ns.name];
                              const visibleConfigMaps = normalizedQuery
                                ? configmaps?.filter((cm) => {
                                    const q = normalizedQuery;
                                    return [cm.name].filter(Boolean).some((value) => value.toLowerCase().includes(q));
                                  })
                                : configmaps;
                              const displayCount =
                                normalizedQuery && configmaps ? visibleConfigMaps?.length || 0 : count;
                              return (
                                <div key={rt.key}>
                                  <div
                                    className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                    onClick={() => toggleConfigMaps(ns.name)}
                                  >
                                    {configMapsExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    )}
                                    <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                    <span className="truncate">{t(rt.labelKey)}</span>
                                    <span className="ml-auto text-[10px] text-muted-foreground">{displayCount}</span>
                                  </div>
                                  {configMapsExpanded && (
                                    <div className="ml-3">
                                      {loadingConfigMaps.has(ns.name) && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("asset.k8sLoadingConfigMaps")}
                                        </div>
                                      )}
                                      {configMapErrors[ns.name] && (
                                        <div
                                          className="flex items-start gap-1 pl-12 pr-2 py-1 text-xs text-destructive cursor-pointer"
                                          title={configMapErrors[ns.name]}
                                          onClick={() => {
                                            const next = { ...configMapErrors };
                                            delete next[ns.name];
                                            setConfigMapErrors(next);
                                            loadConfigMaps(ns.name);
                                          }}
                                        >
                                          <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                          <span>{t("asset.k8sNamespaceResourceError")}</span>
                                        </div>
                                      )}
                                      {visibleConfigMaps?.length === 0 && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          {t("asset.k8sNoConfigMaps")}
                                        </div>
                                      )}
                                      {visibleConfigMaps?.map((cm) => (
                                        <div
                                          key={cm.name}
                                          className={`flex items-center gap-1.5 pl-12 pr-2 py-1 rounded-md text-xs cursor-pointer ml-1 ${
                                            activeTabId === `cm:${ns.name}:${cm.name}`
                                              ? "bg-muted font-medium"
                                              : "hover:bg-muted/50"
                                          }`}
                                          onClick={() => openTab(`cm:${ns.name}:${cm.name}`, cm.name)}
                                        >
                                          <FileText className="h-3 w-3 shrink-0 text-muted-foreground" />
                                          <span className="truncate">{cm.name}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              );
                            }
                            if (isSecrets) {
                              const secrets = namespaceSecretList[ns.name];
                              const visibleSecrets = normalizedQuery
                                ? secrets?.filter((s) => {
                                    const q = normalizedQuery;
                                    return [s.name, s.type]
                                      .filter(Boolean)
                                      .some((value) => value.toLowerCase().includes(q));
                                  })
                                : secrets;
                              const displayCount = normalizedQuery && secrets ? visibleSecrets?.length || 0 : count;
                              return (
                                <div key={rt.key}>
                                  <div
                                    className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                    onClick={() => toggleSecrets(ns.name)}
                                  >
                                    {secretsExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                                    )}
                                    <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                    <span className="truncate">{t(rt.labelKey)}</span>
                                    <span className="ml-auto text-[10px] text-muted-foreground">{displayCount}</span>
                                  </div>
                                  {secretsExpanded && (
                                    <div className="ml-3">
                                      {loadingSecrets.has(ns.name) && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("asset.k8sLoadingSecrets")}
                                        </div>
                                      )}
                                      {secretErrors[ns.name] && (
                                        <div
                                          className="flex items-start gap-1 pl-12 pr-2 py-1 text-xs text-destructive cursor-pointer"
                                          title={secretErrors[ns.name]}
                                          onClick={() => {
                                            const next = { ...secretErrors };
                                            delete next[ns.name];
                                            setSecretErrors(next);
                                            loadSecrets(ns.name);
                                          }}
                                        >
                                          <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                                          <span>{t("asset.k8sNamespaceResourceError")}</span>
                                        </div>
                                      )}
                                      {visibleSecrets?.length === 0 && (
                                        <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                          {t("asset.k8sNoSecrets")}
                                        </div>
                                      )}
                                      {visibleSecrets?.map((s) => (
                                        <div
                                          key={s.name}
                                          className={`flex items-center gap-1.5 pl-12 pr-2 py-1 rounded-md text-xs cursor-pointer ml-1 ${
                                            activeTabId === `secret:${ns.name}:${s.name}`
                                              ? "bg-muted font-medium"
                                              : "hover:bg-muted/50"
                                          }`}
                                          onClick={() => openTab(`secret:${ns.name}:${s.name}`, s.name)}
                                        >
                                          <Key className="h-3 w-3 shrink-0 text-muted-foreground" />
                                          <span className="truncate">{s.name}</span>
                                          <span className="ml-auto text-[10px] text-muted-foreground">{s.type}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              );
                            }
                            return (
                              <div
                                key={rt.key}
                                className="flex items-center gap-1.5 pl-8 pr-2 py-1 rounded-md text-xs cursor-pointer hover:bg-muted/50"
                                onClick={() => openTab(`ns-res:${ns.name}:${rt.key}`, `${rt.key} (${ns.name})`)}
                              >
                                <rt.icon className="h-3 w-3 shrink-0 text-muted-foreground" style={{}} />
                                <span className="truncate">{t(rt.labelKey)}</span>
                                <span className="ml-auto text-[10px] text-muted-foreground">{count}</span>
                              </div>
                            );
                          })}
                        </>
                      );
                    })()}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div
        className="w-[3px] shrink-0 cursor-col-resize hover:bg-ring/40 active:bg-ring/60 transition-colors"
        onMouseDown={handleSidebarResize}
      />
      {sidebarResizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}

      <div className="flex-1 min-w-0 flex flex-col h-full">
        {innerTabs.length > 0 && (
          <div className="flex items-center border-b border-border bg-muted/30 shrink-0 overflow-x-auto">
            {innerTabs.map((tab) => {
              const isActive = tab.id === activeTabId;
              return (
                <div
                  key={tab.id}
                  className={`flex items-center gap-1.5 px-3 py-1.5 text-xs cursor-pointer border-r border-border whitespace-nowrap select-none transition-colors duration-150 ${
                    isActive ? "bg-background border-b-2 border-b-primary -mb-[1px] font-medium" : "hover:bg-muted/50"
                  }`}
                  onClick={() => setActiveTabId(tab.id)}
                >
                  {tab.id === "overview" ? (
                    <Server className="h-3 w-3" />
                  ) : tab.id.startsWith("node:") ? (
                    <Box className="h-3 w-3" />
                  ) : tab.id.startsWith("pod:") ? (
                    <Circle className="h-3 w-3" />
                  ) : tab.id.startsWith("svc:") ? (
                    <Container className="h-3 w-3" />
                  ) : tab.id.startsWith("cm:") ? (
                    <FileText className="h-3 w-3" />
                  ) : tab.id.startsWith("secret:") ? (
                    <Key className="h-3 w-3" />
                  ) : tab.id.startsWith("ns-res:") ? (
                    (() => {
                      const resType = RESOURCE_TYPES.find((rt) => tab.id.endsWith(`:${rt.key}`));
                      if (resType) return <resType.icon className="h-3 w-3" style={{}} />;
                      return <Layers className="h-3 w-3" />;
                    })()
                  ) : (
                    <Layers className="h-3 w-3" />
                  )}
                  {tab.label}
                  {tab.id !== "overview" && (
                    <button
                      className="ml-1 rounded-sm hover:bg-muted-foreground/20 p-0.5"
                      onClick={(e) => {
                        e.stopPropagation();
                        closeTab(tab.id);
                      }}
                    >
                      <svg width="10" height="10" viewBox="0 0 10 10" className="text-muted-foreground">
                        <line x1="2" y1="2" x2="8" y2="8" stroke="currentColor" strokeWidth="1.2" />
                        <line x1="8" y1="2" x2="2" y2="8" stroke="currentColor" strokeWidth="1.2" />
                      </svg>
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        )}

        <div className="flex-1 overflow-y-auto">
          {activeTabId === "overview" && (
            <div className="max-w-5xl mx-auto p-4 space-y-4">
              <K8sSectionCard>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
                  <InfoItem label={t("asset.k8sVersion")} value={info.version} mono />
                  <InfoItem label={t("asset.k8sPlatform")} value={info.platform} mono />
                  <InfoItem label={t("asset.k8sNodes")} value={String(info.nodes.length)} mono />
                </div>
              </K8sSectionCard>

              <K8sSectionCard title={t("asset.k8sNodes")}>
                <div className="grid gap-3 sm:grid-cols-2">
                  {info.nodes.map((node) => (
                    <div
                      key={node.name}
                      className="rounded-lg border p-3 cursor-pointer hover:bg-muted/30 transition-colors"
                      onClick={() => openTab(`node:${node.name}`, node.name)}
                    >
                      <div className="flex items-center justify-between mb-2">
                        <span className="font-mono text-sm font-medium">{node.name}</span>
                        <span
                          className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusVariantToClass(getK8sStatusColor(node.status))}`}
                        >
                          {node.status === "True" ? "Ready" : node.status}
                        </span>
                      </div>
                      <div className="grid grid-cols-2 gap-1 text-xs text-muted-foreground">
                        <span>OS: {node.os}</span>
                        <span>Arch: {node.arch}</span>
                        <span>CPU: {node.cpu}</span>
                        <span>Mem: {node.memory}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </K8sSectionCard>

              <K8sSectionCard title={t("asset.k8sNamespaces")}>
                <div className="flex flex-wrap gap-2">
                  {info.namespaces.map((ns) => (
                    <span
                      key={ns.name}
                      className={`inline-flex items-center rounded-md border px-3 py-1 text-sm font-mono cursor-pointer hover:bg-muted/50 ${
                        ns.status === "Active" ? "" : "text-muted-foreground border-dashed"
                      }`}
                      onClick={() => openTab(`ns:${ns.name}`, ns.name)}
                    >
                      {ns.name}
                    </span>
                  ))}
                </div>
              </K8sSectionCard>
            </div>
          )}

          {activeNode && (
            <div className="max-w-5xl mx-auto p-4 space-y-4">
              <K8sSectionCard>
                <K8sResourceHeader
                  name={activeNode.name}
                  status={{
                    text: activeNode.status === "True" ? "Ready" : activeNode.status,
                    variant: getK8sStatusColor(activeNode.status),
                  }}
                />
                <K8sMetadataGrid
                  items={[
                    { label: "OS", value: activeNode.os, mono: true },
                    { label: "Architecture", value: activeNode.arch, mono: true },
                    { label: "Kubernetes", value: `v${activeNode.version}`, mono: true },
                    { label: "CPU", value: activeNode.cpu, mono: true },
                    { label: "Memory", value: activeNode.memory, mono: true },
                    { label: "Roles", value: activeNode.roles.join(", "), mono: true },
                  ]}
                />
              </K8sSectionCard>
            </div>
          )}

          {activeNs && (
            <div className="max-w-5xl mx-auto p-4 space-y-4">
              <K8sSectionCard>
                <K8sResourceHeader
                  name={activeNs.name}
                  subtitle={`${t("asset.k8sNamespace")}: ${activeNs.status}`}
                  status={{
                    text: activeNs.status,
                    variant: activeNs.status === "Active" ? "success" : "neutral",
                  }}
                />
                {loadingNamespaces.has(activeNs.name) ? (
                  <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("asset.k8sLoadingNamespace")}
                  </div>
                ) : namespaceErrors[activeNs.name] ? (
                  <div
                    className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive cursor-pointer"
                    onClick={() => {
                      const next = { ...namespaceErrors };
                      delete next[activeNs.name];
                      setNamespaceErrors(next);
                      loadNamespaceResources(activeNs.name);
                    }}
                  >
                    <div className="flex items-center gap-2">
                      <AlertCircle className="h-4 w-4" />
                      {t("asset.k8sNamespaceResourceError")}
                    </div>
                    <p className="text-xs mt-1 opacity-70">{namespaceErrors[activeNs.name]}</p>
                  </div>
                ) : namespaceResources[activeNs.name] ? (
                  <K8sMetadataGrid
                    items={RESOURCE_TYPES.map((rt) => {
                      const count = namespaceResources[activeNs.name][rt.key] as number;
                      return {
                        label: t(rt.labelKey),
                        value: String(count),
                        mono: true,
                      };
                    })}
                  />
                ) : null}
              </K8sSectionCard>
            </div>
          )}

          {activeTabId.startsWith("ns-res:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const resKey = parts[2];
              const rt = RESOURCE_TYPES.find((r) => r.key === resKey);
              const res = namespaceResources[ns];
              const count = res ? (res[resKey as keyof NamespaceResourcesData] as number) : 0;
              return (
                <div className="max-w-5xl mx-auto p-4 space-y-4">
                  <K8sSectionCard>
                    <div className="flex items-center gap-3 mb-4">
                      {rt && <rt.icon className="h-5 w-5 text-muted-foreground" style={{}} />}
                      <h3 className="font-mono text-sm font-medium">{rt ? t(rt.labelKey) : resKey}</h3>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">{ns}</span>
                    </div>
                    <K8sMetadataGrid
                      items={[{ label: t("asset.k8sNamespaceResources"), value: String(count), mono: true }]}
                    />
                  </K8sSectionCard>
                </div>
              );
            })()}

          {activeTabId.startsWith("pod:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const podName = parts.slice(2).join(":");
              const key = `${ns}/${podName}`;
              const detail = podDetails[key];
              const loading = loadingPodDetails.has(key);
              const err = podDetailErrors[key];

              if (loading) {
                return (
                  <div className="flex items-center justify-center h-full">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                  </div>
                );
              }
              if (err) {
                return (
                  <div className="flex flex-col items-center justify-center h-full gap-4 p-8">
                    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive max-w-md text-center">
                      {err}
                    </div>
                    <button
                      onClick={() => {
                        const next = { ...podDetailErrors };
                        delete next[key];
                        setPodDetailErrors(next);
                        loadPodDetail(ns, podName);
                      }}
                      className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm hover:bg-muted"
                    >
                      <RefreshCw className="h-3.5 w-3.5" />
                      {t("action.retry")}
                    </button>
                  </div>
                );
              }
              if (!detail) return null;

              return (
                <div className="max-w-5xl mx-auto p-4 space-y-4">
                  <K8sSectionCard>
                    <K8sResourceHeader
                      name={detail.name}
                      subtitle={`${detail.namespace} · ${detail.node_name}`}
                      status={{ text: detail.status, variant: getK8sStatusColor(detail.status) }}
                    />
                    <K8sMetadataGrid
                      items={[
                        { label: t("asset.k8sPodIP"), value: detail.pod_ip || "-", mono: true },
                        { label: t("asset.k8sPodHostIP"), value: detail.host_ip || "-", mono: true },
                        { label: t("asset.k8sPodCreationTime"), value: detail.creation_time },
                        { label: t("asset.k8sPodReady"), value: detail.ready, mono: true },
                        { label: t("asset.k8sPodQosClass"), value: detail.qos_class },
                      ]}
                    />
                  </K8sSectionCard>

                  <K8sTableSection
                    title={t("asset.k8sPodContainers")}
                    columns={[
                      { key: "name", label: t("asset.k8sPodName") },
                      { key: "image", label: "Image" },
                      { key: "state", label: t("asset.k8sPodStatus") },
                      { key: "ready", label: t("asset.k8sPodReady") },
                      { key: "restarts", label: t("asset.k8sPodRestarts") },
                    ]}
                    data={detail.containers}
                    emptyText={t("asset.k8sNoEvents")}
                    renderRow={(c) => (
                      <tr key={c.name} className="border-b last:border-0">
                        <td className="py-2 pr-4 font-mono text-sm">{c.name}</td>
                        <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{c.image}</td>
                        <td className="py-2 pr-4">
                          <span
                            className={`text-xs px-1.5 py-0.5 rounded-full ${statusVariantToClass(getContainerStateColor(c.state))}`}
                          >
                            {c.state}
                          </span>
                        </td>
                        <td className="py-2 pr-4">
                          <span className={c.ready ? "text-green-600" : "text-red-600"}>
                            {c.ready ? "\u2713" : "\u2717"}
                          </span>
                        </td>
                        <td className="py-2 font-mono text-sm">{c.restart_count}</td>
                      </tr>
                    )}
                  />

                  <K8sLogsPanel
                    containers={detail.containers}
                    namespace={detail.namespace}
                    podName={detail.name}
                    logContainer={logContainer}
                    logTailLines={logTailLines}
                    logStreamID={logStreamID}
                    logError={logError}
                    logLines={logLines}
                    onContainerChange={(container) => {
                      setLogContainer(container);
                      if (logStreamID) {
                        stopLogStream();
                        startLogStream(detail.namespace, detail.name, container, logTailLines);
                      }
                    }}
                    onTailLinesChange={(lines) => setLogTailLines(lines)}
                    onStart={() => {
                      const container = logContainer || detail.containers[0]?.name || "";
                      startLogStream(detail.namespace, detail.name, container, logTailLines);
                    }}
                    onStop={stopLogStream}
                  />

                  <K8sTableSection
                    title={t("asset.k8sPodEvents")}
                    columns={[
                      { key: "type", label: "Type" },
                      { key: "reason", label: "Reason" },
                      { key: "message", label: "Message" },
                      { key: "count", label: "Count" },
                      { key: "last_time", label: "Last Seen" },
                    ]}
                    data={detail.events}
                    emptyText={t("asset.k8sNoEvents")}
                    renderRow={(e, i) => (
                      <tr key={i} className="border-b last:border-0">
                        <td className="py-2 pr-4">
                          <span
                            className={`text-xs px-1.5 py-0.5 rounded-full ${statusVariantToClass(e.type === "Warning" ? "warning" : "info")}`}
                          >
                            {e.type}
                          </span>
                        </td>
                        <td className="py-2 pr-4 text-xs">{e.reason}</td>
                        <td className="py-2 pr-4 text-xs text-muted-foreground max-w-xs truncate">{e.message}</td>
                        <td className="py-2 pr-4 font-mono text-xs">{e.count}</td>
                        <td className="py-2 text-xs text-muted-foreground">{e.last_time}</td>
                      </tr>
                    )}
                  />

                  <K8sConditionList conditions={detail.conditions} title={t("asset.k8sPodConditions")} defaultCollapsed />

                  <K8sTagList tags={detail.labels} title={t("asset.k8sPodLabels")} defaultCollapsed />

                  <K8sCodeBlock code={detail.yaml} title={t("asset.k8sPodYAML")} defaultCollapsed />
                </div>
              );
            })()}

          {activeTabId.startsWith("svc:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const svcName = parts.slice(2).join(":");
              const svc = namespaceServiceList[ns]?.find((s) => s.name === svcName);

              if (!svc) {
                return (
                  <div className="flex items-center justify-center h-full">
                    <span className="text-sm text-muted-foreground">{t("asset.k8sNoServices")}</span>
                  </div>
                );
              }

              return (
                <div className="max-w-5xl mx-auto p-4 space-y-4">
                  <K8sSectionCard>
                    <K8sResourceHeader
                      name={svc.name}
                      subtitle={svc.namespace}
                      status={{ text: svc.type, variant: "info" }}
                    />
                    <K8sMetadataGrid
                      items={[
                        { label: t("asset.k8sServiceType"), value: svc.type, mono: true },
                        { label: t("asset.k8sServiceClusterIP"), value: svc.cluster_ip || "-", mono: true },
                        { label: t("asset.k8sPodAge"), value: svc.age, mono: true },
                      ]}
                    />
                  </K8sSectionCard>

                  <K8sTableSection
                    title={t("asset.k8sServicePorts")}
                    columns={[
                      { key: "name", label: t("asset.k8sPodName") },
                      { key: "port", label: t("asset.k8sServicePort") },
                      { key: "target_port", label: t("asset.k8sServiceTargetPort") },
                      { key: "protocol", label: t("asset.k8sServiceProtocol") },
                      { key: "node_port", label: "NodePort" },
                    ]}
                    data={svc.ports}
                    emptyText={t("asset.k8sNoEvents")}
                    renderRow={(p, i) => (
                      <tr key={i} className="border-b last:border-0">
                        <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{p.name || "-"}</td>
                        <td className="py-2 pr-4 font-mono text-sm">{p.port}</td>
                        <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{p.target_port || "-"}</td>
                        <td className="py-2 pr-4 text-xs">{p.protocol}</td>
                        <td className="py-2 font-mono text-xs text-muted-foreground">{p.node_port || "-"}</td>
                      </tr>
                    )}
                  />
                </div>
              );
            })()}

          {activeTabId.startsWith("cm:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const cmName = parts.slice(2).join(":");
              const cm = namespaceConfigMapList[ns]?.find((c) => c.name === cmName);

              if (!cm) {
                return (
                  <div className="flex items-center justify-center h-full">
                    <span className="text-sm text-muted-foreground">{t("asset.k8sNoConfigMaps")}</span>
                  </div>
                );
              }

              const dataEntries = Object.entries(cm.data || {});

              return (
                <div className="max-w-5xl mx-auto p-4 space-y-4">
                  <K8sSectionCard>
                    <K8sResourceHeader
                      name={cm.name}
                      subtitle={cm.namespace}
                      status={{
                        text: `${dataEntries.length} key${dataEntries.length !== 1 ? "s" : ""}`,
                        variant: "neutral",
                      }}
                    />
                    <K8sMetadataGrid items={[{ label: t("asset.k8sPodAge"), value: cm.age, mono: true }]} />
                  </K8sSectionCard>

                  <K8sSectionCard title="Data">
                    {dataEntries.length === 0 ? (
                      <p className="text-xs text-muted-foreground">{t("asset.k8sNoEvents")}</p>
                    ) : (
                      <div className="space-y-3">
                        {dataEntries.map(([key, value]) => (
                          <div key={key}>
                            <div className="text-xs text-muted-foreground font-medium mb-1">{key}</div>
                            <K8sCodeBlock code={value} maxHeight="max-h-64" />
                          </div>
                        ))}
                      </div>
                    )}
                  </K8sSectionCard>
                </div>
              );
            })()}

          {activeTabId.startsWith("secret:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const secretName = parts.slice(2).join(":");
              const secret = namespaceSecretList[ns]?.find((s) => s.name === secretName);

              if (!secret) {
                return (
                  <div className="flex items-center justify-center h-full">
                    <span className="text-sm text-muted-foreground">{t("asset.k8sNoSecrets")}</span>
                  </div>
                );
              }

              const dataEntries = Object.entries(secret.data || {});
              const decodeValue = (encoded: string) => {
                try {
                  return atob(encoded);
                } catch {
                  return encoded;
                }
              };

              return (
                <div className="max-w-5xl mx-auto p-4 space-y-4">
                  <K8sSectionCard>
                    <K8sResourceHeader
                      name={secret.name}
                      subtitle={secret.namespace}
                      status={{ text: secret.type, variant: "neutral" }}
                    />
                    <K8sMetadataGrid
                      items={[
                        { label: t("asset.k8sSecretType"), value: secret.type, mono: true },
                        { label: t("asset.k8sPodAge"), value: secret.age, mono: true },
                      ]}
                    />
                  </K8sSectionCard>

                  <K8sSectionCard title={t("asset.k8sSecretData")}>
                    {dataEntries.length === 0 ? (
                      <p className="text-xs text-muted-foreground">{t("asset.k8sNoEvents")}</p>
                    ) : (
                      <div className="space-y-3">
                        {dataEntries.map(([key, value]) => {
                          const decoded = decodeValue(value);
                          return (
                            <div key={key}>
                              <div className="flex items-center justify-between mb-1">
                                <span className="text-xs text-muted-foreground font-medium">{key}</span>
                                <span className="text-[10px] text-muted-foreground">{decoded.length}B</span>
                              </div>
                              <K8sCodeBlock code={decoded} maxHeight="max-h-32" />
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </K8sSectionCard>
                </div>
              );
            })()}
        </div>
      </div>
    </div>
  );
}
