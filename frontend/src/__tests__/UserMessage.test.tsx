/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { UserMessage } from "@/components/ai/UserMessage";
import { useAssetStore } from "@/stores/assetStore";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore } from "@/stores/tabStore";

const editButtonName = /ai\.editMessage|编辑消息|Edit message/i;
const copyButtonName = /action\.copy|复制|Copy/i;

describe("UserMessage", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN");
    localStorage.setItem("language", "zh-CN");
    useAssetStore.setState({
      assets: [
        {
          ID: 42,
          Name: "prod-db",
          Type: "database",
          Icon: "mysql",
          Config: JSON.stringify({ driver: "mysql", database: "app" }),
        } as any,
      ],
      groups: [],
    } as any);
    useQueryStore.setState({ dbStates: {}, redisStates: {}, mongoStates: {} } as any);
    useTabStore.setState({ tabs: [], activeTabId: null } as any);
  });

  it("无 mention 标签时渲染纯文本", () => {
    render(<UserMessage msg={{ role: "user", content: "hello", blocks: [] } as any} />);
    expect(screen.getByText("hello")).toBeInTheDocument();
  });

  it("内联 <mention> XML 解析为可点击 chip", () => {
    const msg = {
      role: "user",
      content: 'check <mention asset-id="42" type="mysql">@prod-db</mention> disk',
      blocks: [],
    } as any;
    render(<UserMessage msg={msg} />);
    expect(screen.getByText(/check/)).toBeInTheDocument();
    const chip = screen.getByRole("button", { name: /prod-db/ });
    expect(chip).toBeInTheDocument();
    expect(screen.getByText(/disk/)).toBeInTheDocument();
  });

  it("点击 chip 打开 info tab", async () => {
    const msg = {
      role: "user",
      content: '<mention asset-id="42">@prod-db</mention>',
      blocks: [],
    } as any;
    render(<UserMessage msg={msg} />);
    await userEvent.click(screen.getByRole("button", { name: /prod-db/ }));
    expect(useTabStore.getState().tabs.some((t) => t.id === "info-asset-42")).toBe(true);
  });

  it("点击表 mention chip 跳转到对应数据库表 tab", async () => {
    const msg = {
      role: "user",
      content:
        '<mention asset-id="42" type="database" target="table" database="app" table="users" driver="mysql">@app.users</mention>',
      blocks: [],
    } as any;

    render(<UserMessage msg={msg} />);
    await userEvent.click(screen.getByRole("button", { name: /app\.users/ }));

    expect(useTabStore.getState().activeTabId).toBe("query-42");
    const dbState = useQueryStore.getState().dbStates["query-42"];
    expect(dbState.activeInnerTabId).toBe("table:app.users");
    expect(dbState.innerTabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ type: "table", database: "app", table: "users" })])
    );
  });

  it("点击库 mention chip 跳转到对应数据库 SQL tab 并展开库", async () => {
    const msg = {
      role: "user",
      content: '<mention asset-id="42" type="database" target="database" database="app" driver="mysql">@app</mention>',
      blocks: [],
    } as any;

    render(<UserMessage msg={msg} />);
    await userEvent.click(screen.getByRole("button", { name: /app/ }));

    expect(useTabStore.getState().activeTabId).toBe("query-42");
    const dbState = useQueryStore.getState().dbStates["query-42"];
    expect(dbState.expandedDbs).toContain("app");
    const activeInnerTab = dbState.innerTabs.find((tab) => tab.id === dbState.activeInnerTabId);
    expect(activeInnerTab).toMatchObject({ type: "sql", selectedDb: "app" });
  });

  it("keeps the copy button when edit is available and triggers onEdit", async () => {
    const onEdit = vi.fn();
    const msg = {
      role: "user",
      content: "需要回改",
      blocks: [],
    } as any;

    render(<UserMessage msg={msg} index={2} onEdit={onEdit} />);

    expect(screen.getByRole("button", { name: editButtonName })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: copyButtonName })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: editButtonName }));
    expect(onEdit).toHaveBeenCalledTimes(1);
    expect(onEdit).toHaveBeenCalledWith(2, msg);
  });
});
