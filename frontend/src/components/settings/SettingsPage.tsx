import { useTranslation } from "react-i18next";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@opskat/ui";
import { Bot, Palette, HardDrive, Import, Keyboard, MonitorDot, Info, Activity, Puzzle } from "lucide-react";
import { ShortcutSettings } from "@/components/settings/ShortcutSettings";
import { AISettingsSection } from "@/components/settings/AISettingsSection";
import { ImportSection } from "@/components/settings/ImportSection";
import { BackupSection } from "@/components/settings/BackupSection";
import { AppearanceSection, TerminalSection } from "@/components/settings/AppearanceSection";
import { UpdateSection } from "@/components/settings/UpdateSection";
import { SystemStatusSection } from "@/components/settings/SystemStatusSection";
import { ExtensionSection } from "@/components/settings/ExtensionSection";

export function SettingsPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col h-full">
      <div className="px-4 py-3 border-b">
        <h2 className="font-semibold">{t("nav.settings")}</h2>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <Tabs defaultValue="ai" className="space-y-4 max-w-4xl mx-auto">
          <TabsList>
            <TabsTrigger value="ai" className="gap-1">
              <Bot className="h-3.5 w-3.5" />
              AI
            </TabsTrigger>
            <TabsTrigger value="import" className="gap-1">
              <Import className="h-3.5 w-3.5" />
              {t("import.title")}
            </TabsTrigger>
            <TabsTrigger value="backup" className="gap-1">
              <HardDrive className="h-3.5 w-3.5" />
              {t("backup.title")}
            </TabsTrigger>
            <TabsTrigger value="shortcuts" className="gap-1">
              <Keyboard className="h-3.5 w-3.5" />
              {t("shortcut.title")}
            </TabsTrigger>
            <TabsTrigger value="terminal" className="gap-1">
              <MonitorDot className="h-3.5 w-3.5" />
              {t("terminal.title")}
            </TabsTrigger>
            <TabsTrigger value="appearance" className="gap-1">
              <Palette className="h-3.5 w-3.5" />
              {t("nav.appearance")}
            </TabsTrigger>
            <TabsTrigger value="status" className="gap-1">
              <Activity className="h-3.5 w-3.5" />
              {t("systemStatus.title")}
            </TabsTrigger>
            <TabsTrigger value="extensions" className="gap-1">
              <Puzzle className="h-3.5 w-3.5" />
              {t("extension.title")}
            </TabsTrigger>
            <TabsTrigger value="about" className="gap-1">
              <Info className="h-3.5 w-3.5" />
              {t("appUpdate.title")}
            </TabsTrigger>
          </TabsList>

          {/* AI Provider */}
          <TabsContent value="ai" className="space-y-4">
            <AISettingsSection />
          </TabsContent>

          {/* Import */}
          <TabsContent value="import" className="space-y-4">
            <ImportSection />
          </TabsContent>

          {/* Backup */}
          <TabsContent value="backup" className="space-y-4">
            <BackupSection />
          </TabsContent>

          {/* Shortcuts */}
          <TabsContent value="shortcuts" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("shortcut.title")}</CardTitle>
                <CardDescription>{t("shortcut.desc")}</CardDescription>
              </CardHeader>
              <CardContent>
                <ShortcutSettings />
              </CardContent>
            </Card>
          </TabsContent>

          {/* Terminal */}
          <TabsContent value="terminal" className="space-y-4">
            <TerminalSection />
          </TabsContent>

          {/* Appearance and Language */}
          <TabsContent value="appearance" className="space-y-4">
            <AppearanceSection />
          </TabsContent>

          {/* About & Update */}
          <TabsContent value="about" className="space-y-4">
            <UpdateSection />
          </TabsContent>

          {/* System Status */}
          <TabsContent value="status" className="space-y-4">
            <SystemStatusSection />
          </TabsContent>

          {/* Extensions */}
          <TabsContent value="extensions" className="space-y-4">
            <ExtensionSection />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
