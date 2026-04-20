import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { EdgeRevealStrip } from "@/components/layout/EdgeRevealStrip";

describe("EdgeRevealStrip", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders a button", () => {
    render(<EdgeRevealStrip onClick={vi.fn()} />);
    const strip = screen.getByRole("button");
    expect(strip).toBeInTheDocument();
  });

  it("calls onClick when clicked", () => {
    const onClick = vi.fn();
    render(<EdgeRevealStrip onClick={onClick} />);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("defaults to left side with showSidebar tooltip", () => {
    render(<EdgeRevealStrip onClick={vi.fn()} />);
    const strip = screen.getByRole("button");
    expect(strip).toHaveClass("left-0");
    expect(strip).toHaveAttribute("title", "panel.showSidebar");
  });

  it("renders on the right with showAIPanel tooltip when side=right", () => {
    render(<EdgeRevealStrip onClick={vi.fn()} side="right" />);
    const strip = screen.getByRole("button");
    expect(strip).toHaveClass("right-0");
    expect(strip).toHaveAttribute("title", "panel.showAIPanel");
  });
});
