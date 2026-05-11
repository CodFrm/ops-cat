/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { UserMessage } from "@/components/ai/UserMessage";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";

const editButtonName = /ai\.editMessage|编辑消息|Edit message/i;
const copyButtonName = /action\.copy|复制|Copy/i;

describe("UserMessage", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("zh-CN");
    localStorage.setItem("language", "zh-CN");
    useAssetStore.setState({
      assets: [{ ID: 42, Name: "prod-db", Type: "mysql", Icon: "mysql" } as any],
      groups: [],
    } as any);
    useTabStore.setState({ tabs: [], activeTabId: null } as any);
  });

  it("无 mentions 时渲染纯文本", () => {
    render(<UserMessage index={0} msg={{ role: "user", content: "hello", blocks: [] } as any} />);
    expect(screen.getByText("hello")).toBeInTheDocument();
  });

  it("有 mentions 时按 start/end 切片渲染 chip", () => {
    const msg = {
      role: "user",
      content: "check @prod-db disk",
      mentions: [{ assetId: 42, name: "prod-db", start: 6, end: 14 }],
      blocks: [],
    } as any;
    render(<UserMessage index={0} msg={msg} />);
    expect(screen.getByText(/check/)).toBeInTheDocument();
    const chip = screen.getByRole("button", { name: /prod-db/ });
    expect(chip).toBeInTheDocument();
    expect(screen.getByText(/disk/)).toBeInTheDocument();
  });

  it("点击 chip 打开 info tab", async () => {
    const msg = {
      role: "user",
      content: "@prod-db",
      mentions: [{ assetId: 42, name: "prod-db", start: 0, end: 8 }],
      blocks: [],
    } as any;
    render(<UserMessage index={0} msg={msg} />);
    await userEvent.click(screen.getByRole("button", { name: /prod-db/ }));
    expect(useTabStore.getState().tabs.some((t) => t.id === "info-asset-42")).toBe(true);
  });

  it("越界 mention 整体丢弃，不会把短文本吞成 chip", () => {
    // 回归 "继续" 被渲染成 chip 的 bug：content 长度远小于 mention.end，
    // 旧逻辑会 slice(0, 13) 拿到整条 "继续" 渲染为 button。修复后该 mention
    // 视为脏数据丢弃，渲染回纯文本。
    const msg = {
      role: "user",
      content: "继续",
      mentions: [{ assetId: 42, name: "prod-db", start: 0, end: 13 }],
      blocks: [],
    } as any;
    render(<UserMessage index={0} msg={msg} />);
    expect(screen.getByText("继续")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /prod-db/ })).not.toBeInTheDocument();
  });

  it("keeps the copy button when edit is available and triggers onEdit", async () => {
    const onEdit = vi.fn();
    const msg = {
      role: "user",
      content: "需要回改",
      blocks: [],
    } as any;

    render(<UserMessage index={3} msg={msg} onEdit={onEdit} />);

    expect(screen.getByRole("button", { name: editButtonName })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: copyButtonName })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: editButtonName }));
    expect(onEdit).toHaveBeenCalledTimes(1);
    expect(onEdit).toHaveBeenCalledWith(3, msg);
  });
});
