import {StrictMode} from "react";
import {createRoot} from "react-dom/client";
import {CSPProvider} from "@base-ui/react/csp-provider";
import {CaptureApp} from "./capture-app";
import "./styles/shadcn.css";
import "./style.css";

createRoot(document.getElementById("root")!).render(<StrictMode><CSPProvider disableStyleElements><CaptureApp/></CSPProvider></StrictMode>);
