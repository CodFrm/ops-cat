import { useTranslation } from "react-i18next";
import { Eye, EyeOff } from "lucide-react";
import { Button, Input, Label, Textarea } from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";

export interface K8sConfigSectionProps {
  kubeconfig: string;
  setKubeconfig: (v: string) => void;
  showKubeconfig: boolean;
  setShowKubeconfig: (v: boolean) => void;
  namespace: string;
  setNamespace: (v: string) => void;
  contextName: string;
  setContextName: (v: string) => void;
  sshTunnelId: number;
  setSshTunnelId: (v: number) => void;
  isEditing: boolean;
}

export function K8sConfigSection({
  kubeconfig,
  setKubeconfig,
  showKubeconfig,
  setShowKubeconfig,
  namespace,
  setNamespace,
  contextName,
  setContextName,
  sshTunnelId,
  setSshTunnelId,
  isEditing,
}: K8sConfigSectionProps) {
  const { t } = useTranslation();
  const placeholder = isEditing
    ? t("asset.k8sKubeconfigEditPlaceholder")
    : t("asset.k8sKubeconfigPlaceholder");
  return (
    <div className="grid gap-3 border rounded-lg p-4">
      <div className="grid gap-2">
        <Label>{t("asset.k8sKubeconfig")}</Label>
        {showKubeconfig ? (
          <div className="relative min-w-0 overflow-hidden">
            <Textarea
              value={kubeconfig}
              onChange={(e) => setKubeconfig(e.target.value)}
              placeholder={placeholder}
              rows={4}
              className="font-mono text-xs pr-9 whitespace-pre-wrap break-all"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-1 top-2 h-7 w-7"
              onClick={() => setShowKubeconfig(false)}
            >
              <EyeOff className="h-3.5 w-3.5" />
            </Button>
          </div>
        ) : (
          <Button type="button" variant="outline" className="w-full" onClick={() => setShowKubeconfig(true)}>
            <Eye className="h-3.5 w-3.5 mr-1" />
            {isEditing ? t("asset.k8sRevealKubeconfig") : t("asset.k8sEnterKubeconfig")}
          </Button>
        )}
      </div>
      <div className="grid gap-2">
        <Label>{t("asset.k8sNamespace")}</Label>
        <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="default" />
      </div>
      <div className="grid gap-2">
        <Label>{t("asset.k8sContext")}</Label>
        <Input value={contextName} onChange={(e) => setContextName(e.target.value)} placeholder="current context" />
      </div>
      <div className="grid gap-2">
        <Label>{t("asset.sshTunnel")}</Label>
        <AssetSelect
          value={sshTunnelId}
          onValueChange={setSshTunnelId}
          filterType="ssh"
          placeholder={t("asset.sshTunnelNone")}
        />
      </div>
    </div>
  );
}
