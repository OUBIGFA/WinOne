// Compatibility names keep existing actions stable during the shadcn migration.
// Rendering and focus styles come entirely from the official component.
import type {ComponentProps} from "react";
import {LoaderCircle,type LucideIcon} from "lucide-react";
import {Button as ShadcnButton} from "../components/ui/button";

type ButtonProps = Omit<ComponentProps<typeof ShadcnButton>, "variant"> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  busy?: boolean;
  icon?: LucideIcon;
};

export function Button({
  className = "",
  variant = "secondary",
  busy = false,
  disabled,
  children,
  icon: Icon,
  ...props
}: ButtonProps) {
  return (
    <ShadcnButton
      {...props}
      className={`max-w-full h-auto min-h-8 whitespace-normal ${className}`}
      variant={variant==="primary"?"default":variant==="danger"?"destructive":variant==="secondary"?"outline":"ghost"}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
    >
      {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : Icon && <Icon className="size-4" aria-hidden="true"/>}
      {children}
    </ShadcnButton>
  );
}
