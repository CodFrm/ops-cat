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
  HardDrive,
  UserCheck,
  AlertCircle,
  ChevronRight,
  ChevronDown,
  Play,
  Square,
  ScrollText,
} from "lucide-react";
import type { asset_entity } from "../../../wailsjs/go/models";
import {
  GetK8sClusterInfo,
  GetK8sNamespaceResources,
  GetK8sNamespacePods,
  GetK8sPodDetail,
  StartK8sPodLogs,
  StopK8sPodLogs,
} from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";

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
  | `pod:${string}:${string}`;

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
  { key: "pvcs", labelKey: "asset.k8sPVCs", icon: HardDrive },
  { key: "service_accounts", labelKey: "asset.k8sServiceAccounts", icon: UserCheck },
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
  const [namespaceResources, setNamespaceResources] = useState<Record<string, NamespaceResourcesData>>({});
  const [loadingNamespaces, setLoadingNamespaces] = useState<Set<string>>(new Set());
  const [namespaceErrors, setNamespaceErrors] = useState<Record<string, string>>({});
  const [namespacePodList, setNamespacePodList] = useState<Record<string, PodListItem[]>>({});
  const [loadingPods, setLoadingPods] = useState<Set<string>>(new Set());
  const [podErrors, setPodErrors] = useState<Record<string, string>>({});
  const [podDetails, setPodDetails] = useState<Record<string, PodDetail>>({});
  const [loadingPodDetails, setLoadingPodDetails] = useState<Set<string>>(new Set());
  const [podDetailErrors, setPodDetailErrors] = useState<Record<string, string>>({});
  const [logStreamID, setLogStreamID] = useState<string | null>(null);
  const [logLines, setLogLines] = useState<string[]>([]);
  const [logContainer, setLogContainer] = useState("");
  const [logTailLines, setLogTailLines] = useState(200);
  const [logError, setLogError] = useState<string | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const logStreamIDRef = useRef<string | null>(null);

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
        setNamespaceResources({});
        setLoadingNamespaces(new Set());
        setNamespaceErrors({});
        setNamespacePodList({});
        setLoadingPods(new Set());
        setPodErrors({});
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

  return (
    <div className="flex h-full w-full">
      <div className="shrink-0 w-52 border-r border-border bg-sidebar h-full overflow-y-auto">
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
                    RESOURCE_TYPES.map((rt) => {
                      const count = namespaceResources[ns.name][rt.key] as number;
                      const isPods = rt.key === "pods";
                      const podsExpanded = expandedPods.has(ns.name);
                      if (isPods) {
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
                              <span className="ml-auto text-[10px] text-muted-foreground">{count}</span>
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
                                {namespacePodList[ns.name]?.length === 0 && (
                                  <div className="flex items-center gap-1.5 pl-12 pr-2 py-1 text-xs text-muted-foreground">
                                    {t("asset.k8sNoPods")}
                                  </div>
                                )}
                                {namespacePodList[ns.name]?.map((pod) => (
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
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="w-[3px] shrink-0 cursor-col-resize hover:bg-ring/40 active:bg-ring/60 transition-colors" />

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
            <div className="max-w-4xl mx-auto p-6 space-y-6">
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
                <div className="rounded-lg bg-muted/50 p-4">
                  <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sVersion")}</div>
                  <div className="text-lg font-mono font-semibold">{info.version}</div>
                </div>
                <div className="rounded-lg bg-muted/50 p-4">
                  <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPlatform")}</div>
                  <div className="text-lg font-mono font-semibold">{info.platform}</div>
                </div>
                <div className="rounded-lg bg-muted/50 p-4">
                  <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sNodes")}</div>
                  <div className="text-lg font-mono font-semibold">{info.nodes.length}</div>
                </div>
              </div>

              <div className="rounded-xl border bg-card p-6">
                <h3 className="text-sm font-semibold mb-3">{t("asset.k8sNodes")}</h3>
                <div className="grid gap-3 sm:grid-cols-2">
                  {info.nodes.map((node) => (
                    <div
                      key={node.name}
                      className="rounded-lg border p-4 cursor-pointer hover:bg-muted/30 transition-colors"
                      onClick={() => openTab(`node:${node.name}`, node.name)}
                    >
                      <div className="flex items-center justify-between mb-2">
                        <span className="font-mono text-sm font-medium">{node.name}</span>
                        <span
                          className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                            node.status === "True"
                              ? "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400"
                              : "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400"
                          }`}
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
              </div>

              <div className="rounded-xl border bg-card p-6">
                <h3 className="text-sm font-semibold mb-3">{t("asset.k8sNamespaces")}</h3>
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
              </div>
            </div>
          )}

          {activeNode && (
            <div className="max-w-4xl mx-auto p-6 space-y-6">
              <div className="rounded-xl border bg-card p-6">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-base font-semibold">{activeNode.name}</h3>
                  <span
                    className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                      activeNode.status === "True"
                        ? "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400"
                        : "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400"
                    }`}
                  >
                    {activeNode.status === "True" ? "Ready" : activeNode.status}
                  </span>
                </div>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">OS</div>
                    <div className="font-mono font-medium">{activeNode.os}</div>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">Architecture</div>
                    <div className="font-mono font-medium">{activeNode.arch}</div>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">Kubernetes</div>
                    <div className="font-mono font-medium">v{activeNode.version}</div>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">CPU</div>
                    <div className="font-mono font-medium">{activeNode.cpu}</div>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">Memory</div>
                    <div className="font-mono font-medium">{activeNode.memory}</div>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-4">
                    <div className="text-xs text-muted-foreground mb-1">Roles</div>
                    <div className="font-mono font-medium">{activeNode.roles.join(", ")}</div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeNs && (
            <div className="max-w-4xl mx-auto p-6 space-y-6">
              <div className="rounded-xl border bg-card p-6">
                <h3 className="text-base font-semibold mb-1">{activeNs.name}</h3>
                <p className="text-xs text-muted-foreground mb-4">
                  {t("asset.k8sNamespace")}:{" "}
                  <span className={activeNs.status === "Active" ? "text-green-600" : ""}>{activeNs.status}</span>
                </p>
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
                  <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                    {RESOURCE_TYPES.map((rt) => {
                      const count = namespaceResources[activeNs.name][rt.key] as number;
                      return (
                        <div
                          key={rt.key}
                          className="rounded-lg border p-3 cursor-pointer hover:bg-muted/30 transition-colors"
                          onClick={() => openTab(`ns-res:${activeNs.name}:${rt.key}`, `${rt.key} (${activeNs.name})`)}
                        >
                          <div className="flex items-center gap-2 mb-1">
                            <rt.icon className="h-4 w-4 text-muted-foreground" style={{}} />
                            <span className="text-sm font-medium">{t(rt.labelKey)}</span>
                          </div>
                          <span className="text-2xl font-mono font-semibold">{count}</span>
                        </div>
                      );
                    })}
                  </div>
                ) : null}
              </div>
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
                <div className="max-w-4xl mx-auto p-6 space-y-6">
                  <div className="rounded-xl border bg-card p-6">
                    <div className="flex items-center gap-3 mb-4">
                      {rt && <rt.icon className="h-5 w-5 text-muted-foreground" style={{}} />}
                      <h3 className="text-base font-semibold">{rt ? t(rt.labelKey) : resKey}</h3>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">{ns}</span>
                    </div>
                    <div className="rounded-lg bg-muted/50 p-4">
                      <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sNamespaceResources")}</div>
                      <div className="text-2xl font-mono font-semibold">{count}</div>
                    </div>
                  </div>
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

              const getStatusColor = (status: string) => {
                if (status === "Running") return "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400";
                if (status === "Pending")
                  return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-400";
                return "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400";
              };

              const getContainerStateColor = (state: string) => {
                if (state.startsWith("Running"))
                  return "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400";
                if (state.startsWith("Waiting"))
                  return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-400";
                return "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400";
              };

              return (
                <div className="max-w-5xl mx-auto p-6 space-y-6">
                  <div className="rounded-xl border bg-card p-6">
                    <div className="flex items-center justify-between mb-4">
                      <div>
                        <h3 className="text-base font-semibold font-mono">{detail.name}</h3>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {detail.namespace} &middot; {detail.node_name}
                        </p>
                      </div>
                      <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${getStatusColor(detail.status)}`}>
                        {detail.status}
                      </span>
                    </div>

                    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
                      <div className="rounded-lg bg-muted/50 p-3">
                        <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPodIP")}</div>
                        <div className="font-mono text-sm font-medium">{detail.pod_ip || "-"}</div>
                      </div>
                      <div className="rounded-lg bg-muted/50 p-3">
                        <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPodHostIP")}</div>
                        <div className="font-mono text-sm font-medium">{detail.host_ip || "-"}</div>
                      </div>
                      <div className="rounded-lg bg-muted/50 p-3">
                        <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPodCreationTime")}</div>
                        <div className="text-sm font-medium">{detail.creation_time}</div>
                      </div>
                      <div className="rounded-lg bg-muted/50 p-3">
                        <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPodReady")}</div>
                        <div className="font-mono text-sm font-medium">{detail.ready}</div>
                      </div>
                      <div className="rounded-lg bg-muted/50 p-3">
                        <div className="text-xs text-muted-foreground mb-1">{t("asset.k8sPodQosClass")}</div>
                        <div className="text-sm font-medium">{detail.qos_class}</div>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-xl border bg-card p-6">
                    <h4 className="text-sm font-semibold mb-3">{t("asset.k8sPodContainers")}</h4>
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b">
                            <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">
                              {t("asset.k8sPodName")}
                            </th>
                            <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">Image</th>
                            <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">
                              {t("asset.k8sPodStatus")}
                            </th>
                            <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">
                              {t("asset.k8sPodReady")}
                            </th>
                            <th className="text-left py-2 text-xs text-muted-foreground font-medium">
                              {t("asset.k8sPodRestarts")}
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          {detail.containers.map((c) => (
                            <tr key={c.name} className="border-b last:border-0">
                              <td className="py-2 pr-4 font-mono">{c.name}</td>
                              <td className="py-2 pr-4 font-mono text-muted-foreground">{c.image}</td>
                              <td className="py-2 pr-4">
                                <span
                                  className={`text-xs px-1.5 py-0.5 rounded-full ${getContainerStateColor(c.state)}`}
                                >
                                  {c.state}
                                </span>
                              </td>
                              <td className="py-2 pr-4">
                                <span className={c.ready ? "text-green-600" : "text-red-600"}>
                                  {c.ready ? "\u2713" : "\u2717"}
                                </span>
                              </td>
                              <td className="py-2 font-mono">{c.restart_count}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div className="rounded-xl border bg-card p-6">
                    <h4 className="text-sm font-semibold mb-3">{t("asset.k8sPodConditions")}</h4>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      {detail.conditions.map((c) => (
                        <div key={c.type} className="rounded-lg border p-3">
                          <div className="flex items-center justify-between mb-1">
                            <span className="text-sm font-medium">{c.type}</span>
                            <span
                              className={`text-xs px-1.5 py-0.5 rounded-full ${
                                c.status === "True"
                                  ? "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400"
                                  : "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400"
                              }`}
                            >
                              {c.status}
                            </span>
                          </div>
                          {c.reason && <p className="text-xs text-muted-foreground">{c.reason}</p>}
                          {c.message && <p className="text-xs text-muted-foreground mt-0.5">{c.message}</p>}
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="rounded-xl border bg-card p-6">
                    <h4 className="text-sm font-semibold mb-3">{t("asset.k8sPodEvents")}</h4>
                    {detail.events.length === 0 ? (
                      <p className="text-xs text-muted-foreground">{t("asset.k8sNoEvents")}</p>
                    ) : (
                      <div className="overflow-x-auto max-h-64 overflow-y-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b">
                              <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">Type</th>
                              <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">Reason</th>
                              <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">Message</th>
                              <th className="text-left py-2 pr-4 text-xs text-muted-foreground font-medium">Count</th>
                              <th className="text-left py-2 text-xs text-muted-foreground font-medium">Last Seen</th>
                            </tr>
                          </thead>
                          <tbody>
                            {detail.events.map((e, i) => (
                              <tr key={i} className="border-b last:border-0">
                                <td className="py-2 pr-4">
                                  <span
                                    className={`text-xs px-1.5 py-0.5 rounded-full ${
                                      e.type === "Warning"
                                        ? "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-400"
                                        : "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-400"
                                    }`}
                                  >
                                    {e.type}
                                  </span>
                                </td>
                                <td className="py-2 pr-4 text-xs">{e.reason}</td>
                                <td className="py-2 pr-4 text-xs text-muted-foreground max-w-xs truncate">
                                  {e.message}
                                </td>
                                <td className="py-2 pr-4 font-mono text-xs">{e.count}</td>
                                <td className="py-2 text-xs text-muted-foreground">{e.last_time}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  {Object.keys(detail.labels).length > 0 && (
                    <div className="rounded-xl border bg-card p-6">
                      <h4 className="text-sm font-semibold mb-3">{t("asset.k8sPodLabels")}</h4>
                      <div className="flex flex-wrap gap-2">
                        {Object.entries(detail.labels).map(([k, v]) => (
                          <span
                            key={k}
                            className="inline-flex items-center rounded-md border bg-muted/50 px-2 py-0.5 text-xs font-mono"
                          >
                            {k}: {v}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}

                  <div className="rounded-xl border bg-card p-6">
                    <h4 className="text-sm font-semibold mb-3">{t("asset.k8sPodYAML")}</h4>
                    <pre className="bg-muted/50 rounded-lg p-4 text-xs font-mono max-h-96 overflow-y-auto whitespace-pre-wrap">
                      {detail.yaml}
                    </pre>
                  </div>

                  <div className="rounded-xl border bg-card p-6">
                    <div className="flex items-center justify-between mb-3">
                      <h4 className="text-sm font-semibold flex items-center gap-2">
                        <ScrollText className="h-4 w-4" />
                        {t("asset.k8sPodLogs")}
                      </h4>
                      <div className="flex items-center gap-2">
                        <select
                          className="h-7 rounded-md border bg-background px-2 text-xs"
                          value={logContainer || detail.containers[0]?.name || ""}
                          onChange={(e) => {
                            setLogContainer(e.target.value);
                            if (logStreamID) {
                              stopLogStream();
                              startLogStream(detail.namespace, detail.name, e.target.value, logTailLines);
                            }
                          }}
                          disabled={!!logStreamID}
                        >
                          {detail.containers.map((c) => (
                            <option key={c.name} value={c.name}>
                              {c.name}
                            </option>
                          ))}
                        </select>
                        <input
                          type="number"
                          className="h-7 w-16 rounded-md border bg-background px-2 text-xs"
                          value={logTailLines}
                          onChange={(e) => setLogTailLines(Number(e.target.value))}
                          disabled={!!logStreamID}
                          min={1}
                          max={10000}
                          title={t("asset.k8sPodLogsTailLines")}
                        />
                        {logStreamID ? (
                          <button
                            onClick={stopLogStream}
                            className="inline-flex items-center gap-1.5 rounded-md border border-destructive/50 px-3 py-1.5 text-xs text-destructive hover:bg-destructive/10"
                          >
                            <Square className="h-3 w-3" />
                            {t("asset.k8sPodLogsStop")}
                          </button>
                        ) : (
                          <button
                            onClick={() => {
                              const container = logContainer || detail.containers[0]?.name || "";
                              startLogStream(detail.namespace, detail.name, container, logTailLines);
                            }}
                            className="inline-flex items-center gap-1.5 rounded-md border border-primary/50 px-3 py-1.5 text-xs text-primary hover:bg-primary/10"
                          >
                            <Play className="h-3 w-3" />
                            {t("asset.k8sPodLogsStart")}
                          </button>
                        )}
                      </div>
                    </div>
                    {logError && (
                      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive mb-3">
                        {t("asset.k8sPodLogsError")}: {logError}
                      </div>
                    )}
                    <div className="bg-black rounded-lg p-4 text-xs font-mono max-h-96 overflow-y-auto">
                      {logLines.length === 0 && !logStreamID && !logError && (
                        <span className="text-gray-500">{t("asset.k8sPodLogsStopped")}</span>
                      )}
                      {logStreamID && logLines.length === 0 && (
                        <span className="text-gray-500">{t("asset.k8sPodLogsStreaming")}</span>
                      )}
                      {logLines.map((line, i) => (
                        <span key={i} className="text-green-400">
                          {line}
                        </span>
                      ))}
                      <div ref={logEndRef} />
                    </div>
                  </div>
                </div>
              );
            })()}
        </div>
      </div>
    </div>
  );
}
