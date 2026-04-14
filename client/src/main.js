import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import "./assets/styles/global.css";

const app = createApp(App);
app.use(createPinia());

app.config.errorHandler = (err, instance, info) => {
  console.error("[Vue Error]", info, err);
};

app.mount("#app");
