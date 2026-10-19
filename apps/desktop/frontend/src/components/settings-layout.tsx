import {useId, useRef, type CSSProperties, type ReactNode, type ComponentProps} from "react";
import {ChevronRight, type LucideIcon} from "lucide-react";
import {cn} from "cn";
import {IconLabel} from "../ui/icon-label";
import {FieldGroup} from "./ui/field";
import {Label} from "./ui/label";
import {RadioGroup, RadioGroupItem} from "./ui/radio-group";
import {Switch} from "./ui/switch";

// Compose the bundled shadcn FieldGroup rather than introduce another control
// system. A heading owns its fields; descriptions stay close to that heading.
// Every group directly below a tab takes an icon, so a page reads as one set of
// named tasks instead of mixing labelled and unlabelled headings.
export function SettingsSection({title, description, icon, children, className, level = 3, headingId, ...attributes}: {
  title: ReactNode;
  description?: ReactNode;
  icon?: LucideIcon;
  children: ReactNode;
  className?: string;
  level?: 2 | 3;
  headingId?: string;
} & Omit<ComponentProps<"section">,"title"|"children">) {
  const generatedId = useId(), id = headingId ?? generatedId;
  const Heading = level === 2 ? "h2" : "h3";
  return <section {...attributes} className={cn("settings-group settings-section-block", className)} aria-labelledby={id}>
    <div className="settings-group-heading">
      <Heading id={id} className="settings-group-title">{icon ? <IconLabel icon={icon}>{title}</IconLabel> : title}</Heading>
      {description && <p className="settings-group-description">{description}</p>}
    </div>
    <FieldGroup className="settings-group-body">{children}</FieldGroup>
  </section>;
}

// Both short segments and explanatory choice rows use the bundled radio, not
// tab buttons. Descriptions are announced separately from the stable label.
export function SettingsChoice({id, value, children, description, disabled}: {
  id: string;
  value: string;
  children: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
}) {
  return <Label className="settings-choice" htmlFor={id}>
    <RadioGroupItem id={id} value={value} disabled={disabled}
      aria-labelledby={`${id}-label`} aria-describedby={description ? `${id}-description` : undefined}/>
    <span className="settings-choice-copy">
      <span id={`${id}-label`} className="settings-choice-label">{children}</span>
      {description && <span id={`${id}-description`} className="settings-choice-description">{description}</span>}
    </span>
  </Label>;
}

// Pick presentation by the decision, never by page or current viewport. Short
// comparable choices share a compact track; consequences need readable rows.
// CSS reflows the same mounted controls, retaining every option and focus.
export function SettingsOptions<Value extends string>({label, labelledBy, value, items, disabled, onChange, presentation = "segmented", hint, id}: {
  label: ReactNode;
  labelledBy?: string;
  value: Value;
  items: readonly {value: Value; label: ReactNode; description?: ReactNode; disabled?: boolean; id?: string}[];
  disabled?: boolean;
  onChange: (value: Value) => void;
  presentation?: "segmented" | "list";
  hint?: ReactNode;
  id?: string;
}) {
  const generatedId = useId(), prefix = id ?? generatedId;
  const labelId = labelledBy ?? `${prefix}-label`;
  return <div className="settings-option-field">
    {!labelledBy && <div id={labelId} className="settings-field-label">{label}</div>}
    <RadioGroup id={id} aria-labelledby={labelId} aria-describedby={hint ? `${prefix}-hint` : undefined}
      value={value} disabled={disabled} onValueChange={next => onChange(next as Value)}
      className="settings-options" data-presentation={presentation}
      style={{"--settings-option-count": items.length} as CSSProperties}>
      {items.map((item, index) => <SettingsChoice key={item.value} id={item.id ?? `${prefix}-${index}`}
        value={item.value} description={item.description} disabled={item.disabled}>{item.label}</SettingsChoice>)}
    </RadioGroup>
    {hint && <p id={`${prefix}-hint`} className="settings-help">{hint}</p>}
  </div>;
}

// A binary preference keeps its label, optional explanation and hit target in
// one shared row. Business pages still own the draft and commit semantics.
export function SettingsToggle({id, label, description, checked, disabled, onChange}: {
  id?: string;
  label: ReactNode;
  description?: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onChange: (value: boolean) => void;
}) {
  const generatedId = useId(), controlId = id ?? generatedId;
  return <div className="settings-switch-row">
    <div className="settings-row-copy">
      <Label htmlFor={controlId}>{label}</Label>
      {description && <p id={`${controlId}-description`} className="settings-help">{description}</p>}
    </div>
    <Switch id={controlId} checked={checked} disabled={disabled} onCheckedChange={value => onChange(value)}
      aria-describedby={description ? `${controlId}-description` : undefined}/>
  </div>;
}

function SettingsHelp({title, children}: {title: ReactNode; children: ReactNode}) {
  const id = useId();
  // Short explanatory content stays readable in place, not behind an icon
  // tile or another disclosure. This is help, never a substitute for controls.
  return <div className="settings-inline-help" role="note" aria-labelledby={id}>
    <p id={id} className="settings-inline-help-title">{title}</p>
    {children}
  </div>;
}

export {SettingsHelp};

// Native disclosure keeps its contents mounted, preserving in-progress fields.
// Only optional help / infrequent controls belong here, never errors or risks.
export function SettingsDisclosure({title, children, defaultOpen = false, className}: {
  title: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
  className?: string;
}) {
  const initiallyOpen = useRef(defaultOpen).current;
  return <details className={cn("settings-disclosure", className)} open={initiallyOpen || undefined}>
    <summary><ChevronRight aria-hidden="true" className="settings-disclosure-chevron"/>{title}</summary>
    <div className="settings-disclosure-content">{children}</div>
  </details>;
}
