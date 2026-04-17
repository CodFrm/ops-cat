import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, fireEvent, cleanup } from "@testing-library/react";
import { LeftPanel } from "../../../components/layout/LeftPanel";
import { useLayoutStore } from "../../../stores/layoutStore";

describe("LeftPanel", () => {
  beforeEach(() => {
    useLayoutStore.setState({
      tabBarLayout: "left",
      leftPanelWidth: 220,
      leftPanelVisible: true,
      activeSidePanel: "assets",
      filterOpen: false,
    });
  });

  afterEach(() => {
    cleanup();
  });

  it("renders children", () => {
    const { getByText } = render(
      <LeftPanel>
        <span>hello</span>
      </LeftPanel>
    );
    expect(getByText("hello")).toBeInTheDocument();
  });

  it("width reflects store", () => {
    const { container } = render(
      <LeftPanel>
        <span />
      </LeftPanel>
    );
    const outer = container.firstChild as HTMLElement;
    expect(outer.style.width).toBe("220px");
  });

  it("renders at MIN_PANEL_WIDTH when collapsed", () => {
    useLayoutStore.setState({ leftPanelWidth: 80 });
    const { container } = render(
      <LeftPanel>
        <span />
      </LeftPanel>
    );
    const outer = container.firstChild as HTMLElement;
    expect(outer.style.width).toBe("48px");
  });

  it("drag updates store width", () => {
    const { container } = render(
      <LeftPanel>
        <span />
      </LeftPanel>
    );
    const handle = container.querySelector(".cursor-col-resize") as HTMLElement;
    expect(handle).toBeTruthy();

    fireEvent.mouseDown(handle, { clientX: 300 });
    fireEvent(document, new MouseEvent("mousemove", { clientX: 400 }));
    fireEvent(document, new MouseEvent("mouseup"));

    expect(useLayoutStore.getState().leftPanelWidth).toBe(320);
  });

  it("drag below threshold triggers collapse", () => {
    const { container } = render(
      <LeftPanel>
        <span />
      </LeftPanel>
    );
    const handle = container.querySelector(".cursor-col-resize") as HTMLElement;

    fireEvent.mouseDown(handle, { clientX: 300 });
    fireEvent(document, new MouseEvent("mousemove", { clientX: 100 }));
    fireEvent(document, new MouseEvent("mouseup"));

    expect(useLayoutStore.getState().leftPanelWidth).toBe(48);
  });
});
