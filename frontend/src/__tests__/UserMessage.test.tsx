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

  it("keeps the copy button when edit is available and triggers onEdit", async () => {
    const onEdit = vi.fn();
    const msg = {
      role: "user",
      content: "需要回改",
      blocks: [],
    } as any;

    render(<UserMessage msg={msg} onEdit={onEdit} />);

    expect(screen.getByRole("button", { name: editButtonName })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: copyButtonName })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: editButtonName }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });
});
