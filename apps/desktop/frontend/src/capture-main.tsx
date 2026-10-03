import {StrictMode} from "react";
import {createRoot} from "react-dom/client";
import {CaptureApp} from "./capture-app";
import "./styles/shadcn.css";
import "./style.css";

createRoot(document.getElementById("root")!).render(<StrictMode><CaptureApp/></StrictMode>);
