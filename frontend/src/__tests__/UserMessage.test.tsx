import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UserMessage } from "@/components/ai/UserMessage";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore } from "@/stores/tabStore";

describe("UserMessage", () => {
  beforeEach(() => {
    useAssetStore.setState({
      assets: [{ ID: 42, Name: "prod-db", Type: "mysql", Icon: "mysql" } as any],
      groups: [],
    } as any);
    useTabStore.setState({ tabs: [], activeTabId: null } as any);
  });

  it("无 mentions 时渲染纯文本", () => {
    render(<UserMessage msg={{ role: "user", content: "hello", blocks: [] } as any} />);
    expect(screen.getByText("hello")).toBeInTheDocument();
  });

  it("有 mentions 时按 start/end 切片渲染 chip", () => {
    const msg = {
      role: "user",
      content: "check @prod-db disk",
      mentions: [{ assetId: 42, name: "prod-db", start: 6, end: 14 }],
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
      content: "@prod-db",
      mentions: [{ assetId: 42, name: "prod-db", start: 0, end: 8 }],
      blocks: [],
    } as any;
    render(<UserMessage msg={msg} />);
    await userEvent.click(screen.getByRole("button", { name: /prod-db/ }));
    expect(useTabStore.getState().tabs.some((t) => t.id === "info-asset-42")).toBe(true);
  });
});
