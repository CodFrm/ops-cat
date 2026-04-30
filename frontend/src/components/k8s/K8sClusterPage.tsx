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
  ScrollText,
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
} from "../../../wailsjs/go/app/App";
import {
  Input,
  useResizeHandle,
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@opskat/ui";
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
  | `secret:${string}:${string}`
  | `log:${string}:${string}`
  | `log-deploy:${string}:${string}`;

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

interface K8sPageSnapshot {
  info: ClusterInfo | null;
  innerTabs: InnerTab[];
  activeTabId: InnerTabId;
  expandedNodes: boolean;
  expandedNamespaces: string[];
  expandedPods: string[];
  expandedDeployments: string[];
  expandedServices: string[];
  expandedConfigMaps: string[];
  expandedSecrets: string[];
  expandedDeploymentItems: string[];
  resourceSearch: Record<string, string>;
  namespaceResources: Record<string, NamespaceResourcesData>;
  namespacePodList: Record<string, PodListItem[]>;
  namespaceDeploymentList: Record<string, DeploymentListItem[]>;
  namespaceServiceList: Record<string, ServiceListItem[]>;
  namespaceConfigMapList: Record<string, ConfigMapListItem[]>;
  namespaceSecretList: Record<string, SecretListItem[]>;
  podDetails: Record<string, PodDetail>;
  logTabStates: Record<string, import("./K8sLogsPanel").LogTabState>;
  autoRefreshingItems: string[];
}

interface ResourceSearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

function ResourceSearchInput({ value, onChange, placeholder }: ResourceSearchInputProps) {
  return (
    <div className="relative my-1 ml-9 mr-2">
      <Search className="absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-muted-foreground" />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-6 w-full pl-7 text-xs"
      />
    </div>
  );
}

const k8sPageStateCache = new Map<number, K8sPageSnapshot>();

export function K8sClusterPage({ asset }: Props) {
  const { t } = useTranslation();
  const initialSnapshot = k8sPageStateCache.get(asset.ID);
  const defaultNamespace = (() => {
    try {
      const cfg = JSON.parse(asset.Config || "{}") as { namespace?: string };
      return (cfg.namespace || "").trim();
    } catch {
      return "";
    }
  })();
  const [loading, setLoading] = useState(!initialSnapshot);
  const [refreshing, setRefreshing] = useState(false);
  const [info, setInfo] = useState<ClusterInfo | null>(initialSnapshot?.info || null);
  const [error, setError] = useState<string | null>(null);
  const [innerTabs, setInnerTabs] = useState<InnerTab[]>(
    initialSnapshot?.innerTabs || [{ id: "overview", label: t("asset.k8sClusterOverview") }]
  );
  const [activeTabId, setActiveTabId] = useState<InnerTabId>(initialSnapshot?.activeTabId || "overview");
  const [expandedNodes, setExpandedNodes] = useState(initialSnapshot?.expandedNodes || false);
  const [expandedNamespaces, setExpandedNamespaces] = useState<Set<string>>(
    new Set(initialSnapshot?.expandedNamespaces || [])
  );
  const [expandedPods, setExpandedPods] = useState<Set<string>>(new Set(initialSnapshot?.expandedPods || []));
  const [expandedDeployments, setExpandedDeployments] = useState<Set<string>>(
    new Set(initialSnapshot?.expandedDeployments || [])
  );
  const [expandedServices, setExpandedServices] = useState<Set<string>>(
    new Set(initialSnapshot?.expandedServices || [])
  );
  const [expandedConfigMaps, setExpandedConfigMaps] = useState<Set<string>>(
    new Set(initialSnapshot?.expandedConfigMaps || [])
  );
  const [expandedSecrets, setExpandedSecrets] = useState<Set<string>>(new Set(initialSnapshot?.expandedSecrets || []));
  const [expandedDeploymentItems, setExpandedDeploymentItems] = useState<Set<string>>(
    new Set(initialSnapshot?.expandedDeploymentItems || [])
  );
  const [resourceSearch, setResourceSearch] = useState<Record<string, string>>(initialSnapshot?.resourceSearch || {});
  const [namespaceResources, setNamespaceResources] = useState<Record<string, NamespaceResourcesData>>(
    initialSnapshot?.namespaceResources || {}
  );
  const [loadingNamespaces, setLoadingNamespaces] = useState<Set<string>>(new Set());
  const [namespaceErrors, setNamespaceErrors] = useState<Record<string, string>>({});
  const [namespacePodList, setNamespacePodList] = useState<Record<string, PodListItem[]>>(
    initialSnapshot?.namespacePodList || {}
  );
  const [loadingPods, setLoadingPods] = useState<Set<string>>(new Set());
  const [podErrors, setPodErrors] = useState<Record<string, string>>({});
  const [namespaceDeploymentList, setNamespaceDeploymentList] = useState<Record<string, DeploymentListItem[]>>(
    initialSnapshot?.namespaceDeploymentList || {}
  );
  const [loadingDeployments, setLoadingDeployments] = useState<Set<string>>(new Set());
  const [deploymentErrors, setDeploymentErrors] = useState<Record<string, string>>({});
  const [namespaceServiceList, setNamespaceServiceList] = useState<Record<string, ServiceListItem[]>>(
    initialSnapshot?.namespaceServiceList || {}
  );
  const [loadingServices, setLoadingServices] = useState<Set<string>>(new Set());
  const [serviceErrors, setServiceErrors] = useState<Record<string, string>>({});
  const [namespaceConfigMapList, setNamespaceConfigMapList] = useState<Record<string, ConfigMapListItem[]>>(
    initialSnapshot?.namespaceConfigMapList || {}
  );
  const [loadingConfigMaps, setLoadingConfigMaps] = useState<Set<string>>(new Set());
  const [configMapErrors, setConfigMapErrors] = useState<Record<string, string>>({});
  const [namespaceSecretList, setNamespaceSecretList] = useState<Record<string, SecretListItem[]>>(
    initialSnapshot?.namespaceSecretList || {}
  );
  const [loadingSecrets, setLoadingSecrets] = useState<Set<string>>(new Set());
  const [secretErrors, setSecretErrors] = useState<Record<string, string>>({});
  const [podDetails, setPodDetails] = useState<Record<string, PodDetail>>(initialSnapshot?.podDetails || {});
  const [loadingPodDetails, setLoadingPodDetails] = useState<Set<string>>(new Set());
  const [podDetailErrors, setPodDetailErrors] = useState<Record<string, string>>({});
  const [logTabStates, setLogTabStates] = useState<Record<string, import("./K8sLogsPanel").LogTabState>>(
    initialSnapshot?.logTabStates || {}
  );
  const [refreshingItems, setRefreshingItems] = useState<Set<string>>(new Set());
  const [autoRefreshingItems, setAutoRefreshingItems] = useState<Set<string>>(
    new Set(initialSnapshot?.autoRefreshingItems || [])
  );
  const sidebarRef = useRef<HTMLDivElement>(null);
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

  const loadInfo = (resetState = !initialSnapshot) => {
    if (resetState || !info) {
      setLoading(true);
    }
    setError(null);
    GetK8sClusterInfo(asset.ID)
      .then((result: string) => {
        const data = JSON.parse(result) as ClusterInfo;
        setInfo(data);
        if (resetState) {
          const hasDefaultNamespace = defaultNamespace && data.namespaces.some((ns) => ns.name === defaultNamespace);
          if (hasDefaultNamespace) {
            setInnerTabs([
              { id: "overview", label: t("asset.k8sClusterOverview") },
              { id: `ns:${defaultNamespace}`, label: defaultNamespace },
            ]);
            setActiveTabId(`ns:${defaultNamespace}`);
            setExpandedNamespaces(new Set([defaultNamespace]));
          } else {
            setInnerTabs([{ id: "overview", label: t("asset.k8sClusterOverview") }]);
            setActiveTabId("overview");
            setExpandedNamespaces(new Set());
          }
          setExpandedNodes(false);
          setExpandedPods(new Set());
          setExpandedDeployments(new Set());
          setExpandedServices(new Set());
          setExpandedConfigMaps(new Set());
          setExpandedSecrets(new Set());
          setExpandedDeploymentItems(new Set());
          setResourceSearch({});
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
          setAutoRefreshingItems(new Set());
        }
      })
      .catch((e: unknown) => {
        setError(String(e));
      })
      .finally(() => {
        setLoading(false);
      });
  };

  const refreshInfo = () => {
    setRefreshing(true);
    setAutoRefreshingItems(new Set());
    const promises: Promise<unknown>[] = [];

    promises.push(
      GetK8sClusterInfo(asset.ID)
        .then((result: string) => {
          const data = JSON.parse(result) as ClusterInfo;
          setInfo(data);
        })
        .catch((e: unknown) => {
          setError(String(e));
        })
    );

    // 同时刷新已加载的 namespace 资源数据
    for (const ns of Object.keys(namespaceResources)) {
      setLoadingNamespaces((prev) => new Set(prev).add(ns));
      promises.push(
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
          })
      );
    }

    Promise.all(promises).finally(() => {
      setRefreshing(false);
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

  const refreshDeploymentItem = useCallback(
    (ns: string, deploymentName: string) => {
      const itemKey = `deploy:${ns}/${deploymentName}`;
      if (refreshingItems.has(itemKey)) return;
      setRefreshingItems((prev) => new Set(prev).add(itemKey));
      GetK8sNamespaceDeployments(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as DeploymentListItem[];
          const updated = data.find((d) => d.name === deploymentName);
          if (updated) {
            setNamespaceDeploymentList((prev) => {
              const list = prev[ns] || [];
              const idx = list.findIndex((d) => d.name === deploymentName);
              if (idx >= 0) {
                const newList = [...list];
                newList[idx] = updated;
                return { ...prev, [ns]: newList };
              }
              return prev;
            });
          }
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
          setRefreshingItems((prev) => {
            const next = new Set(prev);
            next.delete(itemKey);
            return next;
          });
        });
    },
    [asset.ID, refreshingItems]
  );

  const refreshPodItem = useCallback(
    (ns: string, podName: string) => {
      const itemKey = `pod:${ns}/${podName}`;
      if (refreshingItems.has(itemKey)) return;
      setRefreshingItems((prev) => new Set(prev).add(itemKey));
      GetK8sNamespacePods(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as PodListItem[];
          const updated = data.find((p) => p.name === podName);
          if (updated) {
            setNamespacePodList((prev) => {
              const list = prev[ns] || [];
              const idx = list.findIndex((p) => p.name === podName);
              if (idx >= 0) {
                const newList = [...list];
                newList[idx] = updated;
                return { ...prev, [ns]: newList };
              }
              return prev;
            });
          }
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
          setRefreshingItems((prev) => {
            const next = new Set(prev);
            next.delete(itemKey);
            return next;
          });
        });
    },
    [asset.ID, refreshingItems]
  );

  const refreshServiceItem = useCallback(
    (ns: string, svcName: string) => {
      const itemKey = `svc:${ns}/${svcName}`;
      if (refreshingItems.has(itemKey)) return;
      setRefreshingItems((prev) => new Set(prev).add(itemKey));
      GetK8sNamespaceServices(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as ServiceListItem[];
          const updated = data.find((s) => s.name === svcName);
          if (updated) {
            setNamespaceServiceList((prev) => {
              const list = prev[ns] || [];
              const idx = list.findIndex((s) => s.name === svcName);
              if (idx >= 0) {
                const newList = [...list];
                newList[idx] = updated;
                return { ...prev, [ns]: newList };
              }
              return prev;
            });
          }
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
          setRefreshingItems((prev) => {
            const next = new Set(prev);
            next.delete(itemKey);
            return next;
          });
        });
    },
    [asset.ID, refreshingItems]
  );

  const refreshConfigMapItem = useCallback(
    (ns: string, cmName: string) => {
      const itemKey = `cm:${ns}/${cmName}`;
      if (refreshingItems.has(itemKey)) return;
      setRefreshingItems((prev) => new Set(prev).add(itemKey));
      GetK8sNamespaceConfigMaps(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as ConfigMapListItem[];
          const updated = data.find((c) => c.name === cmName);
          if (updated) {
            setNamespaceConfigMapList((prev) => {
              const list = prev[ns] || [];
              const idx = list.findIndex((c) => c.name === cmName);
              if (idx >= 0) {
                const newList = [...list];
                newList[idx] = updated;
                return { ...prev, [ns]: newList };
              }
              return prev;
            });
          }
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
          setRefreshingItems((prev) => {
            const next = new Set(prev);
            next.delete(itemKey);
            return next;
          });
        });
    },
    [asset.ID, refreshingItems]
  );

  const refreshSecretItem = useCallback(
    (ns: string, secretName: string) => {
      const itemKey = `secret:${ns}/${secretName}`;
      if (refreshingItems.has(itemKey)) return;
      setRefreshingItems((prev) => new Set(prev).add(itemKey));
      GetK8sNamespaceSecrets(asset.ID, ns)
        .then((result: string) => {
          const data = JSON.parse(result) as SecretListItem[];
          const updated = data.find((s) => s.name === secretName);
          if (updated) {
            setNamespaceSecretList((prev) => {
              const list = prev[ns] || [];
              const idx = list.findIndex((s) => s.name === secretName);
              if (idx >= 0) {
                const newList = [...list];
                newList[idx] = updated;
                return { ...prev, [ns]: newList };
              }
              return prev;
            });
          }
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
          setRefreshingItems((prev) => {
            const next = new Set(prev);
            next.delete(itemKey);
            return next;
          });
        });
    },
    [asset.ID, refreshingItems]
  );

  const silentReloadPodDetail = useCallback(
    (ns: string, podName: string) => {
      const key = `${ns}/${podName}`;
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
        });
    },
    [asset.ID]
  );

  const toggleAutoRefresh = useCallback(
    (itemKey: string, ns: string, name: string) => {
      if (autoRefreshingItems.has(itemKey)) {
        setAutoRefreshingItems((prev) => {
          const next = new Set(prev);
          next.delete(itemKey);
          return next;
        });
      } else {
        setAutoRefreshingItems((prev) => new Set(prev).add(itemKey));
        const colonIdx = itemKey.indexOf(":");
        const type = itemKey.slice(0, colonIdx);
        switch (type) {
          case "pod":
            refreshPodItem(ns, name);
            silentReloadPodDetail(ns, name);
            break;
          case "deploy":
            refreshDeploymentItem(ns, name);
            break;
          case "svc":
            refreshServiceItem(ns, name);
            break;
          case "cm":
            refreshConfigMapItem(ns, name);
            break;
          case "secret":
            refreshSecretItem(ns, name);
            break;
        }
      }
    },
    [
      autoRefreshingItems,
      refreshPodItem,
      refreshDeploymentItem,
      refreshServiceItem,
      refreshConfigMapItem,
      refreshSecretItem,
      silentReloadPodDetail,
    ]
  );

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
    (ns: string, podName: string, force = false) => {
      const key = `${ns}/${podName}`;
      if (!force && (podDetails[key] || loadingPodDetails.has(key))) return;

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

  const updateLogTabState = useCallback((tabId: string, patch: Partial<import("./K8sLogsPanel").LogTabState>) => {
    setLogTabStates((prev) => {
      const existing = prev[tabId] || { logStreamID: null, logContainer: "", logTailLines: 200, logError: null };
      return { ...prev, [tabId]: { ...existing, ...patch } };
    });
  }, []);

  useEffect(() => {
    loadInfo();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [asset.ID]);

  useEffect(() => {
    k8sPageStateCache.set(asset.ID, {
      info,
      innerTabs,
      activeTabId,
      expandedNodes,
      expandedNamespaces: [...expandedNamespaces],
      expandedPods: [...expandedPods],
      expandedDeployments: [...expandedDeployments],
      expandedServices: [...expandedServices],
      expandedConfigMaps: [...expandedConfigMaps],
      expandedSecrets: [...expandedSecrets],
      expandedDeploymentItems: [...expandedDeploymentItems],
      resourceSearch,
      namespaceResources,
      namespacePodList,
      namespaceDeploymentList,
      namespaceServiceList,
      namespaceConfigMapList,
      namespaceSecretList,
      podDetails,
      logTabStates,
      autoRefreshingItems: [...autoRefreshingItems],
    });
  }, [
    asset.ID,
    info,
    innerTabs,
    activeTabId,
    expandedNodes,
    expandedNamespaces,
    expandedPods,
    expandedDeployments,
    expandedServices,
    expandedConfigMaps,
    expandedSecrets,
    expandedDeploymentItems,
    resourceSearch,
    namespaceResources,
    namespacePodList,
    namespaceDeploymentList,
    namespaceServiceList,
    namespaceConfigMapList,
    namespaceSecretList,
    podDetails,
    logTabStates,
    autoRefreshingItems,
  ]);

  const refreshPodItemRef = useRef(refreshPodItem);
  refreshPodItemRef.current = refreshPodItem;
  const refreshDeploymentItemRef = useRef(refreshDeploymentItem);
  refreshDeploymentItemRef.current = refreshDeploymentItem;
  const refreshServiceItemRef = useRef(refreshServiceItem);
  refreshServiceItemRef.current = refreshServiceItem;
  const refreshConfigMapItemRef = useRef(refreshConfigMapItem);
  refreshConfigMapItemRef.current = refreshConfigMapItem;
  const refreshSecretItemRef = useRef(refreshSecretItem);
  refreshSecretItemRef.current = refreshSecretItem;
  const silentReloadPodDetailRef = useRef(silentReloadPodDetail);
  silentReloadPodDetailRef.current = silentReloadPodDetail;

  useEffect(() => {
    if (autoRefreshingItems.size === 0) return;

    const interval = setInterval(() => {
      autoRefreshingItems.forEach((itemKey) => {
        const colonIdx = itemKey.indexOf(":");
        if (colonIdx === -1) return;
        const slashIdx = itemKey.indexOf("/");
        if (slashIdx === -1) return;
        const type = itemKey.slice(0, colonIdx);
        const ns = itemKey.slice(colonIdx + 1, slashIdx);
        const name = itemKey.slice(slashIdx + 1);

        switch (type) {
          case "pod":
            refreshPodItemRef.current(ns, name);
            silentReloadPodDetailRef.current(ns, name);
            break;
          case "deploy":
            refreshDeploymentItemRef.current(ns, name);
            break;
          case "svc":
            refreshServiceItemRef.current(ns, name);
            break;
          case "cm":
            refreshConfigMapItemRef.current(ns, name);
            break;
          case "secret":
            refreshSecretItemRef.current(ns, name);
            break;
        }
      });
    }, 5000);

    return () => clearInterval(interval);
  }, [autoRefreshingItems]);

  const activeNs =
    info && activeTabId.startsWith("ns:") ? info.namespaces.find((n) => n.name === activeTabId.slice(3)) : null;

  useEffect(() => {
    if (activeNs && !namespaceResources[activeNs.name] && !loadingNamespaces.has(activeNs.name)) {
      loadNamespaceResources(activeNs.name);
    }
  }, [activeNs, namespaceResources, loadingNamespaces, loadNamespaceResources]);

  // Reload active pod detail whenever auto-refresh is active for the current pod tab
  useEffect(() => {
    if (!activeTabId.startsWith("pod:")) return;
    const parts = activeTabId.split(":");
    const ns = parts[1];
    const podName = parts.slice(2).join(":");
    const itemKey = `pod:${ns}/${podName}`;
    if (!autoRefreshingItems.has(itemKey)) return;

    const key = `${ns}/${podName}`;
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
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRefreshingItems, activeTabId]);

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
      loadPodDetail(ns, podName, true);
    }
  };

  const openLogTab = (ns: string, podName: string, container: string) => {
    const id = `log:${ns}:${podName}` as InnerTabId;
    const label = `${t("asset.k8sPodLogs")}: ${podName}`;
    if (!innerTabs.some((t) => t.id === id)) {
      setInnerTabs((prev) => [...prev, { id, label }]);
      setLogTabStates((prev) => ({
        ...prev,
        [id]: {
          logStreamID: null,
          logContainer: container,
          logTailLines: 200,
          logError: null,
        },
      }));
    }
    setActiveTabId(id);
  };

  const closeTab = (id: InnerTabId) => {
    if (id.startsWith("log:") || id.startsWith("log-deploy:")) {
      const state = logTabStates[id];
      if (state?.logStreamID) {
        import("../../../wailsjs/go/app/App").then(({ StopK8sPodLogs }) => {
          StopK8sPodLogs(state.logStreamID!);
        });
      }
      setLogTabStates((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
    }
    const idx = innerTabs.findIndex((t) => t.id === id);
    const next = innerTabs.filter((t) => t.id !== id);
    setInnerTabs(next);
    if (activeTabId === id) {
      const neighbor = innerTabs[idx + 1] || innerTabs[idx - 1];
      setActiveTabId(neighbor?.id || "overview");
    }
  };

  const closeInnerTabs = (ids: InnerTabId[], newActiveId?: InnerTabId) => {
    ids.forEach((id) => {
      if (id.startsWith("log:") || id.startsWith("log-deploy:")) {
        const state = logTabStates[id];
        if (state?.logStreamID) {
          import("../../../wailsjs/go/app/App").then(({ StopK8sPodLogs }) => {
            StopK8sPodLogs(state.logStreamID!);
          });
        }
      }
    });
    setLogTabStates((prev) => {
      const next = { ...prev };
      ids.forEach((id) => {
        delete next[id];
      });
      return next;
    });
    setInnerTabs((prev) => prev.filter((t) => !ids.includes(t.id)));
    if (newActiveId !== undefined) {
      setActiveTabId(newActiveId);
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
          onClick={() => loadInfo()}
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
  const serviceMatchesSearch = (svc: ServiceListItem, query: string) => {
    const normalized = query.toLowerCase();
    return [svc.name, svc.type, svc.cluster_ip]
      .filter(Boolean)
      .some((value) => value.toLowerCase().includes(normalized));
  };
  const configMapMatchesSearch = (cm: ConfigMapListItem, query: string) => {
    return cm.name.toLowerCase().includes(query.toLowerCase());
  };
  const secretMatchesSearch = (s: SecretListItem, query: string) => {
    const normalized = query.toLowerCase();
    return s.name.toLowerCase().includes(normalized) || (s.type || "").toLowerCase().includes(normalized);
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
            onClick={refreshInfo}
            disabled={refreshing}
            className="flex items-center gap-1.5 w-full rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:bg-muted/50 mb-1 disabled:opacity-50"
          >
            <RefreshCw className={`h-3 w-3 ${refreshing ? "animate-spin" : ""}`} />
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
                      return (
                        <>
                          {RESOURCE_TYPES.filter((rt) => (namespaceResources[ns.name][rt.key] as number) > 0).map(
                            (rt) => {
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
                                const deploymentsQuery = (resourceSearch[`deployments:${ns.name}`] || "")
                                  .trim()
                                  .toLowerCase();
                                const visibleDeployments = deploymentsQuery
                                  ? deployments?.filter((deployment) =>
                                      deploymentMatchesSearch(deployment, deploymentsQuery)
                                    )
                                  : deployments;
                                const displayCount =
                                  deploymentsQuery && deployments ? visibleDeployments?.length || 0 : count;
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
                                        <ResourceSearchInput
                                          value={resourceSearch[`deployments:${ns.name}`] || ""}
                                          onChange={(v) =>
                                            setResourceSearch((prev) => ({
                                              ...prev,
                                              [`deployments:${ns.name}`]: v,
                                            }))
                                          }
                                          placeholder={t("asset.search")}
                                        />
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
                                          const visiblePods = deploymentsQuery
                                            ? deployment.pods.filter((pod) => podMatchesSearch(pod, deploymentsQuery))
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
                                                <button
                                                  className="ml-1 inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                                  onClick={(e) => {
                                                    e.stopPropagation();
                                                    const firstPod = deployment.pods[0]?.name || "";
                                                    const id = `log-deploy:${ns.name}:${deployment.name}` as InnerTabId;
                                                    const label = `${t("asset.k8sPodLogs")}: ${deployment.name}`;
                                                    if (!innerTabs.some((t) => t.id === id)) {
                                                      setInnerTabs((prev) => [...prev, { id, label }]);
                                                      setLogTabStates((prev) => ({
                                                        ...prev,
                                                        [id]: {
                                                          logStreamID: null,
                                                          logContainer: "",
                                                          logTailLines: 200,
                                                          logError: null,
                                                          currentPod: firstPod,
                                                        },
                                                      }));
                                                    }
                                                    setActiveTabId(id);
                                                    if (firstPod) {
                                                      loadPodDetail(ns.name, firstPod);
                                                    }
                                                  }}
                                                  title={t("asset.k8sPodLogs")}
                                                >
                                                  <ScrollText className="h-3 w-3" />
                                                </button>
                                                <button
                                                  className="ml-0.5 inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                                  onClick={(e) => {
                                                    e.stopPropagation();
                                                    toggleAutoRefresh(
                                                      `deploy:${ns.name}/${deployment.name}`,
                                                      ns.name,
                                                      deployment.name
                                                    );
                                                  }}
                                                  title={t("action.refresh")}
                                                >
                                                  <RefreshCw
                                                    className={`h-3 w-3 ${refreshingItems.has(`deploy:${ns.name}/${deployment.name}`) || autoRefreshingItems.has(`deploy:${ns.name}/${deployment.name}`) ? "animate-spin" : ""}`}
                                                  />
                                                </button>
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
                                                      <button
                                                        className="ml-auto inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                                        onClick={(e) => {
                                                          e.stopPropagation();
                                                          toggleAutoRefresh(
                                                            `pod:${ns.name}/${pod.name}`,
                                                            ns.name,
                                                            pod.name
                                                          );
                                                        }}
                                                        title={t("action.refresh")}
                                                      >
                                                        <RefreshCw
                                                          className={`h-3 w-3 ${refreshingItems.has(`pod:${ns.name}/${pod.name}`) || autoRefreshingItems.has(`pod:${ns.name}/${pod.name}`) ? "animate-spin" : ""}`}
                                                        />
                                                      </button>
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
                                const podsQuery = (resourceSearch[`pods:${ns.name}`] || "").trim().toLowerCase();
                                const visiblePods = podsQuery
                                  ? pods?.filter((pod) => podMatchesSearch(pod, podsQuery))
                                  : pods;
                                const displayCount = podsQuery && pods ? visiblePods?.length || 0 : count;
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
                                        <ResourceSearchInput
                                          value={resourceSearch[`pods:${ns.name}`] || ""}
                                          onChange={(v) =>
                                            setResourceSearch((prev) => ({
                                              ...prev,
                                              [`pods:${ns.name}`]: v,
                                            }))
                                          }
                                          placeholder={t("asset.search")}
                                        />
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
                                            <button
                                              className="ml-auto inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                              onClick={(e) => {
                                                e.stopPropagation();
                                                toggleAutoRefresh(`pod:${ns.name}/${pod.name}`, ns.name, pod.name);
                                              }}
                                              title={t("action.refresh")}
                                            >
                                              <RefreshCw
                                                className={`h-3 w-3 ${refreshingItems.has(`pod:${ns.name}/${pod.name}`) || autoRefreshingItems.has(`pod:${ns.name}/${pod.name}`) ? "animate-spin" : ""}`}
                                              />
                                            </button>
                                          </div>
                                        ))}
                                      </div>
                                    )}
                                  </div>
                                );
                              }
                              if (isServices) {
                                const services = namespaceServiceList[ns.name];
                                const svcQuery = (resourceSearch[`services:${ns.name}`] || "").trim().toLowerCase();
                                const visibleServices = svcQuery
                                  ? services?.filter((svc) => serviceMatchesSearch(svc, svcQuery))
                                  : services;
                                const displayCount = svcQuery && services ? visibleServices?.length || 0 : count;
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
                                        <ResourceSearchInput
                                          value={resourceSearch[`services:${ns.name}`] || ""}
                                          onChange={(v) =>
                                            setResourceSearch((prev) => ({
                                              ...prev,
                                              [`services:${ns.name}`]: v,
                                            }))
                                          }
                                          placeholder={t("asset.search")}
                                        />
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
                                            <span className="ml-auto text-[10px] text-muted-foreground">
                                              {svc.type}
                                            </span>
                                            <button
                                              className="ml-0.5 inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                              onClick={(e) => {
                                                e.stopPropagation();
                                                toggleAutoRefresh(`svc:${ns.name}/${svc.name}`, ns.name, svc.name);
                                              }}
                                              title={t("action.refresh")}
                                            >
                                              <RefreshCw
                                                className={`h-3 w-3 ${refreshingItems.has(`svc:${ns.name}/${svc.name}`) || autoRefreshingItems.has(`svc:${ns.name}/${svc.name}`) ? "animate-spin" : ""}`}
                                              />
                                            </button>
                                          </div>
                                        ))}
                                      </div>
                                    )}
                                  </div>
                                );
                              }
                              if (isConfigMaps) {
                                const configmaps = namespaceConfigMapList[ns.name];
                                const cmQuery = (resourceSearch[`config_maps:${ns.name}`] || "").trim().toLowerCase();
                                const visibleConfigMaps = cmQuery
                                  ? configmaps?.filter((cm) => configMapMatchesSearch(cm, cmQuery))
                                  : configmaps;
                                const displayCount = cmQuery && configmaps ? visibleConfigMaps?.length || 0 : count;
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
                                        <ResourceSearchInput
                                          value={resourceSearch[`config_maps:${ns.name}`] || ""}
                                          onChange={(v) =>
                                            setResourceSearch((prev) => ({
                                              ...prev,
                                              [`config_maps:${ns.name}`]: v,
                                            }))
                                          }
                                          placeholder={t("asset.search")}
                                        />
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
                                            <button
                                              className="ml-auto inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                              onClick={(e) => {
                                                e.stopPropagation();
                                                toggleAutoRefresh(`cm:${ns.name}/${cm.name}`, ns.name, cm.name);
                                              }}
                                              title={t("action.refresh")}
                                            >
                                              <RefreshCw
                                                className={`h-3 w-3 ${refreshingItems.has(`cm:${ns.name}/${cm.name}`) || autoRefreshingItems.has(`cm:${ns.name}/${cm.name}`) ? "animate-spin" : ""}`}
                                              />
                                            </button>
                                          </div>
                                        ))}
                                      </div>
                                    )}
                                  </div>
                                );
                              }
                              if (isSecrets) {
                                const secrets = namespaceSecretList[ns.name];
                                const secretQuery = (resourceSearch[`secrets:${ns.name}`] || "").trim().toLowerCase();
                                const visibleSecrets = secretQuery
                                  ? secrets?.filter((s) => secretMatchesSearch(s, secretQuery))
                                  : secrets;
                                const displayCount = secretQuery && secrets ? visibleSecrets?.length || 0 : count;
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
                                        <ResourceSearchInput
                                          value={resourceSearch[`secrets:${ns.name}`] || ""}
                                          onChange={(v) =>
                                            setResourceSearch((prev) => ({
                                              ...prev,
                                              [`secrets:${ns.name}`]: v,
                                            }))
                                          }
                                          placeholder={t("asset.search")}
                                        />
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
                                            <button
                                              className="ml-0.5 inline-flex items-center gap-1 rounded-sm hover:bg-muted-foreground/20 px-1 py-0.5 text-muted-foreground"
                                              onClick={(e) => {
                                                e.stopPropagation();
                                                toggleAutoRefresh(`secret:${ns.name}/${s.name}`, ns.name, s.name);
                                              }}
                                              title={t("action.refresh")}
                                            >
                                              <RefreshCw
                                                className={`h-3 w-3 ${refreshingItems.has(`secret:${ns.name}/${s.name}`) || autoRefreshingItems.has(`secret:${ns.name}/${s.name}`) ? "animate-spin" : ""}`}
                                              />
                                            </button>
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
                            }
                          )}
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
            {innerTabs.map((tab, idx) => {
              const isActive = tab.id === activeTabId;
              const isOverview = tab.id === "overview";
              const closeableLeftCount = isOverview
                ? 0
                : innerTabs.slice(0, idx).filter((t) => t.id !== "overview").length;
              const closeableRightCount = isOverview ? 0 : innerTabs.slice(idx + 1).length;
              const closeableOthersCount = innerTabs.filter((t) => t.id !== tab.id && t.id !== "overview").length;

              return (
                <ContextMenu key={tab.id}>
                  <ContextMenuTrigger className="contents">
                    <div
                      className={`flex items-center gap-1.5 px-3 py-1.5 text-xs cursor-pointer border-r border-border whitespace-nowrap select-none transition-colors duration-150 ${
                        isActive
                          ? "bg-background border-b-2 border-b-primary -mb-[1px] font-medium"
                          : "hover:bg-muted/50"
                      }`}
                      onClick={() => openTab(tab.id, tab.label)}
                    >
                      {tab.id === "overview" ? (
                        <Server className="h-3 w-3" />
                      ) : tab.id.startsWith("node:") ? (
                        <Box className="h-3 w-3" />
                      ) : tab.id.startsWith("pod:") ? (
                        <Circle className="h-3 w-3" />
                      ) : tab.id.startsWith("log:") ? (
                        <ScrollText className="h-3 w-3" />
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
                      {!isOverview && (
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
                  </ContextMenuTrigger>
                  <ContextMenuContent>
                    {!isOverview && (
                      <ContextMenuItem onClick={() => closeTab(tab.id)}>{t("tab.close")}</ContextMenuItem>
                    )}
                    <ContextMenuItem
                      onClick={() => {
                        const ids = innerTabs.filter((t) => t.id !== tab.id && t.id !== "overview").map((t) => t.id);
                        closeInnerTabs(ids, tab.id);
                      }}
                      disabled={closeableOthersCount === 0}
                    >
                      {t("tab.closeOthers")}
                    </ContextMenuItem>
                    {!isOverview && (
                      <>
                        <ContextMenuSeparator />
                        <ContextMenuItem
                          onClick={() => {
                            const ids = innerTabs
                              .slice(0, idx)
                              .filter((t) => t.id !== "overview")
                              .map((t) => t.id);
                            closeInnerTabs(ids);
                          }}
                          disabled={closeableLeftCount === 0}
                        >
                          {t("tab.closeLeft")}
                        </ContextMenuItem>
                        <ContextMenuItem
                          onClick={() => {
                            const ids = innerTabs.slice(idx + 1).map((t) => t.id);
                            closeInnerTabs(ids);
                          }}
                          disabled={closeableRightCount === 0}
                        >
                          {t("tab.closeRight")}
                        </ContextMenuItem>
                      </>
                    )}
                  </ContextMenuContent>
                </ContextMenu>
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

                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => {
                        const container = detail.containers[0]?.name || "";
                        openLogTab(detail.namespace, detail.name, container);
                      }}
                      className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs hover:bg-muted"
                    >
                      <ScrollText className="h-3 w-3" />
                      {t("asset.k8sPodLogs")}
                    </button>
                  </div>

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

                  <K8sConditionList
                    conditions={detail.conditions}
                    title={t("asset.k8sPodConditions")}
                    defaultCollapsed
                  />

                  <K8sTagList tags={detail.labels} title={t("asset.k8sPodLabels")} defaultCollapsed />

                  <K8sCodeBlock code={detail.yaml} title={t("asset.k8sPodYAML")} defaultCollapsed />
                </div>
              );
            })()}

          {activeTabId.startsWith("log:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const podName = parts.slice(2).join(":");
              const state = logTabStates[activeTabId];
              const detail = podDetails[`${ns}/${podName}`];
              if (!state || !detail) return null;
              return (
                <div className="h-full flex flex-col p-4">
                  <K8sLogsPanel
                    assetId={asset.ID}
                    containers={detail.containers}
                    namespace={ns}
                    podName={podName}
                    state={state}
                    onStateChange={(patch) => updateLogTabState(activeTabId, patch)}
                  />
                </div>
              );
            })()}

          {activeTabId.startsWith("log-deploy:") &&
            (() => {
              const parts = activeTabId.split(":");
              const ns = parts[1];
              const deploymentName = parts.slice(2).join(":");
              const deployment = namespaceDeploymentList[ns]?.find((d) => d.name === deploymentName);
              const state = logTabStates[activeTabId];
              const currentPod = state?.currentPod || deployment?.pods[0]?.name || "";
              const detail = podDetails[`${ns}/${currentPod}`];
              if (!state || !deployment) return null;
              return (
                <div className="h-full flex flex-col p-4">
                  <K8sLogsPanel
                    assetId={asset.ID}
                    containers={detail?.containers || []}
                    namespace={ns}
                    podName={currentPod}
                    state={state}
                    onStateChange={(patch) => updateLogTabState(activeTabId, patch)}
                    pods={deployment.pods}
                    onSwitchPod={(podName) => {
                      updateLogTabState(activeTabId, { currentPod: podName });
                      loadPodDetail(ns, podName);
                    }}
                  />
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
