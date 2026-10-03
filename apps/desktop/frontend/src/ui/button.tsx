// Compatibility names keep existing actions stable during the shadcn migration.
// Rendering and focus styles come entirely from the official component.
import type {ComponentProps} from "react";
import {LoaderCircle} from "lucide-react";
import {Button as ShadcnButton} from "../components/ui/button";

type ButtonProps = Omit<ComponentProps<typeof ShadcnButton>, "variant"> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  busy?: boolean;
};

export function Button({
  className = "",
  variant = "secondary",
  busy = false,
  disabled,
  children,
  ...props
}: ButtonProps) {
  return (
    <ShadcnButton
      {...props}
      className={className}
      variant={variant==="primary"?"default":variant==="danger"?"destructive":variant==="secondary"?"outline":"ghost"}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
    >
      {busy && <LoaderCircle className="animate-spin" aria-hidden="true" />}
      {children}
    </ShadcnButton>
  );
}
