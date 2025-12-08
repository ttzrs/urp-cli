import "./index.css"
import { render } from "solid-js/web"
import { Router, Route } from "@solidjs/router"
import { UrpProvider } from "./context/urp"
import Layout from "./pages/layout"
import Session from "./pages/session"

const root = document.getElementById("root")

render(
  () => (
    <UrpProvider>
      <Router root={Layout}>
        <Route path="/" component={Session} />
        <Route path="/session/:id?" component={Session} />
      </Router>
    </UrpProvider>
  ),
  root!
)
