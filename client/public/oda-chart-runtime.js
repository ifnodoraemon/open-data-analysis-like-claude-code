(function () {
  function showMessage(el, message, color) {
    el.innerHTML =
      '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:' +
      color +
      ';font-size:14px;">' +
      message +
      "</div>";
  }

  function parseOption(el) {
    var raw = el.getAttribute("data-chart-option") || "";
    if (!raw.trim()) return {};
    return JSON.parse(raw);
  }

  function renderCharts() {
    var nodes = document.querySelectorAll(".chart-box[data-chart-id]");
    nodes.forEach(function (el) {
      try {
        if (!window.echarts || typeof window.echarts.init !== "function") {
          showMessage(el, "Chart render failed: ECharts library is not loaded", "#e53e3e");
          return;
        }
        var option = parseOption(el);
        if (!option || typeof option !== "object" || Object.keys(option).length === 0) {
          showMessage(el, "Chart data is empty", "#999");
          return;
        }
        if (!option.tooltip) option.tooltip = { trigger: "axis" };
        if (!option.grid) {
          option.grid = { left: "3%", right: "4%", bottom: "3%", containLabel: true };
        }
        var chart = window.echarts.init(el);
        chart.setOption(option);
        window.addEventListener("resize", function () {
          chart.resize();
        });
      } catch (error) {
        showMessage(el, "Chart render failed: " + error.message, "#e53e3e");
      }
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", renderCharts);
  } else {
    renderCharts();
  }
})();
