import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ExtensionConfigForm } from "@/components/asset/ExtensionConfigForm";

describe("ExtensionConfigForm", () => {
  it('renders format="textarea" as multi-line textarea', () => {
    const schema = {
      type: "object",
      properties: {
        caCert: { type: "string", format: "textarea", title: "CA Certificate" },
      },
    };
    render(<ExtensionConfigForm extensionName="test" configSchema={schema} value={{}} onChange={() => {}} />);
    const el = screen.getByLabelText("CA Certificate");
    expect(el.tagName.toLowerCase()).toBe("textarea");
  });

  it('renders format="password" as masked input', () => {
    const schema = {
      type: "object",
      properties: {
        secret: { type: "string", format: "password", title: "Secret" },
      },
    };
    render(<ExtensionConfigForm extensionName="test" configSchema={schema} value={{}} onChange={() => {}} />);
    const el = screen.getByLabelText("Secret") as HTMLInputElement;
    expect(el.tagName.toLowerCase()).toBe("input");
    expect(el.type).toBe("password");
  });
});
