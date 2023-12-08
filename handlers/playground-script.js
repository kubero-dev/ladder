// Export button
// TODO: Parse JSON to YAML
// TODO: Download YAML
// Injection scripts
// TODO: Javascript syntax highlighting within textarea layout, styling and event listeners
// TODO: Textarea handlers and logic for javascript (e.g. tab, newline)
// TODO: Javascript escaping/unescaping as required
// TODO: remove tailwind play cdn script in head of playground.html after syntax highlighting work complete
// Ninja Keys improvements
// TODO: Group related items for Ninja Keys
// TODO: Untoggle related items that may be toggled (e.g. should only have one masquerade as bot toggled)
// Testing
// TODO: Testing

const modifierContainer = document.getElementById("modifierContainer");
const modalContainer = document.getElementById("modalContainer");
const modalBody = document.getElementById("modal-body");
const modalContent = document.getElementById("modal-content");
const modalSubmitButton = document.getElementById("modal-submit");
const modalClose = document.getElementById("modal-close");

let hasFetched = false;
let payload = {
  requestmodifications: [],
  responsemodifications: [],
};
let ninjaData = [];

initialize();

// Rerun handleThemeChange() so style is applied to Ninja Keys
handleThemeChange();

// Add event listener to the iframe so it closes dropdown when clicked
closeDropdownOnClickWithinIframe();

async function initialize() {
  if (!hasFetched) {
    try {
      await fetchPayload();
      hasFetched = true;
    } catch (error) {
      console.error("Fetch error:", error);
    }
  }
}

function closeDropdownOnClickWithinIframe() {
  const iframe = document.getElementById("resultIframe");
  iframe.contentWindow.document.addEventListener(
    "click",
    () => {
      if (
        !document.getElementById("dropdown_panel").classList.contains("hidden")
      ) {
        toggleDropdown();
      }
    },
    true
  );
}

async function fetchPayload() {
  try {
    const response = await fetch("/api/modifiers");
    const data = await response.json();

    Object.entries(data.result.requestmodifiers ?? []).forEach(([_, value]) => {
      addModifierToNinjaData(
        value.name,
        value.description,
        value.params,
        "requestmodifications"
      );
    });

    Object.entries(data.result.responsemodifiers ?? []).forEach(
      ([_, value]) => {
        addModifierToNinjaData(
          value.name,
          value.description,
          value.params,
          "responsemodifications"
        );
      }
    );

    return data;
  } catch (error) {
    console.error("Fetch error:", error);
    throw error;
  }
}

async function submitForm() {
  if (!document.getElementById("inputForm").checkValidity()) {
    return;
  }

  try {
    const response = await fetch("/playground/" + inputField.value, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      throw new Error("Request failed");
    }

    const result = await response.text();
    updateResultIframe(result);
  } catch (error) {
    console.error(error);
  }
}

function updateResultIframe(result) {
  const resultIframe = parent.document.getElementById("resultIframe");
  resultIframe.contentDocument.open();
  resultIframe.contentDocument.write(result);
  closeDropdownOnClickWithinIframe();
  resultIframe.contentDocument.close();
}

document.getElementById("inputForm").addEventListener("submit", function (e) {
  e.preventDefault();
  submitForm();
});

if (navigator.userAgent.includes("Mac")) {
  document.getElementById("ninjaKey").innerHTML = "⌘";
} else {
  document.getElementById("ninjaKey").innerHTML = "Ctrl";
}

function downloadYaml() {
  function parseYaml() {
    //* PARSE PAYLOAD TO YAML
    return payload;
  }

  const yamlData = parseYaml();
  const blob = new Blob([yamlData], { type: "text/yaml;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  //* CONSTRUCT FILENAME FROM HOSTNAME
  link.download = `name_of_report.yaml`;
  link.click();
  URL.revokeObjectURL(url);
}

function getValues(id, description, params) {
  const focusTrap = trap(modalBody);
  let values = [];
  const inputs = [];
  const inputEventListeners = [];

  function closeModal() {
    focusTrap.destroy();
    modalBody.removeEventListener("keydown", handleEscapeKey);
    modalBody.removeEventListener("keydown", handleEnterKey);
    modalContainer.removeEventListener("click", handleClickOutside);
    modalSubmitButton.removeEventListener("click", closeModal);
    modalClose.removeEventListener("click", closeModal);
    inputEventListeners.forEach((listener, index) => {
      inputs[index].removeEventListener("input", listener);
    });
    inputEventListeners.length = 0;
    inputs.length = 0;
    values = [];
    modalContainer.classList.add("hidden");
    modalContent.innerHTML = "";
  }

  function handleClickOutside(e) {
    if (modalBody !== null && !modalBody.contains(e.target)) {
      closeModal();
    }
  }

  function handleEscapeKey(e) {
    if (e.key === "Escape") {
      closeModal();
    }
  }

  function handleEnterKey(e) {
    if (e.key === "Enter") {
      if (e.target.tagName.toLowerCase() === "textarea") {
        return;
      } else {
        modalSubmitButton.click();
      }
    }
  }

  document.getElementById("modal-title").innerHTML = id;
  document.getElementById("modal-description").innerHTML = description;

  params.map((param, i) => {
    function textareaEventListener(event) {
      codeElement = document.querySelector("code");
      codeElement.innerText = event.target.value;
      Prism.highlightElement(codeElement);
      values[i] = "|" + "&nbsp;" + event.target.value;
    }
    function inputEventListener(event) {
      values[i] = event.target.value;
    }

    const label = document.createElement("label");
    label.innerHTML = param.name;
    label.setAttribute("for", `input-${i}`);
    let input;
    if (param.name === "js") {
      input = document.createElement("textarea");
      input.type = "textarea";
      input.classList.add(
        "min-h-[200px]",
        "font-mono",
        "whitespace-break-spaces",
        "font-semibold"
      );
    } else {
      input = document.createElement("input");
      input.type = "text";
    }
    input.id = `input-${i}`;
    input.classList.add(
      "w-full",
      "text-sm",
      "leading-6",
      "text-slate-400",
      "rounded-md",
      "ring-1",
      "ring-slate-900/10",
      "shadow-sm",
      "py-1.5",
      "pl-2",
      "pr-3",
      "hover:ring-slate-300",
      "dark:bg-slate-800",
      "dark:highlight-white/5"
    );
    modalContent.appendChild(label);
    modalContent.appendChild(input);
    if (input.type === "textarea") {
      preElement = document.createElement("pre");
      codeElement = document.createElement("code");
      preElement.setAttribute("aria-hidden", "true");
      preElement.setAttribute("tabindex", "-1");
      codeElement.classList.add("language-javascript");
      preElement.appendChild(codeElement);
      modalContent.appendChild(preElement);
      input.addEventListener("input", textareaEventListener);
      inputEventListeners.push(textareaEventListener);
    } else {
      input.addEventListener("input", inputEventListener);
      inputEventListeners.push(inputEventListener);
    }
    inputs.push(input);
  });

  modalContainer.classList.remove("hidden");
  document.getElementById("input-0").focus();

  return new Promise((resolve) => {
    modalBody.addEventListener("keydown", handleEscapeKey);
    modalBody.addEventListener("keydown", handleEnterKey);
    modalContainer.addEventListener("click", handleClickOutside);
    modalClose.addEventListener("click", () => {
      closeModal();
    });
    modalSubmitButton.addEventListener("click", () => {
      resolve(values);
      closeModal();
    });
  });
}

function toggleModifier(type, id, params = []) {
  function pillClickHandler(pill) {
    toggleModifier(pill.getAttribute("type"), pill.id);
    pill.removeEventListener("click", () => pillClickHandler(pill));
    pill.remove();
  }

  function createPill(type, id) {
    const pill = document.createElement("span");
    pill.classList.add(
      "inline-flex",
      "items-center",
      "rounded-md",
      "bg-slate-100",
      "dark:bg-slate-800",
      "px-2",
      "py-1",
      "h-4",
      "text-xs",
      "font-medium",
      "border",
      "border-slate-400",
      "dark:border-slate-700",
      "cursor-pointer"
    );
    pill.id = id;
    pill.setAttribute("type", type);
    pill.textContent = id;
    modifierContainer.appendChild(pill);
    pill.addEventListener("click", () => pillClickHandler(pill));
  }

  if (payload[type].some((modifier) => modifier.name === id)) {
    payload[type] = payload[type].filter((modifier) => modifier.name !== id);
    const existingPill = document.getElementById(id);
    if (existingPill !== null) {
      existingPill.removeEventListener("click", () => pillClickHandler(pill));
      existingPill.remove();
    }
  } else {
    payload[type].push({ name: id, params: params });
    createPill(type, id);
  }

  submitForm();
}

function addModifierToNinjaData(id, description, params, type) {
  const section =
    type === "requestmodifications"
      ? "Request Modifiers"
      : "Response Modifiers";
  const modifier = {
    id: id,
    title: id,
    section: section,

    handler: () => {
      if (Object.keys(params).length === 0) {
        toggleModifier(type, id);
      } else {
        if (params[0].name === "_") {
          toggleModifier(type, id, (params = [""]));
        } else {
          getValues(id, description, params).then((values) => {
            if (Object.keys(values).length === 0) return;
            toggleModifier(type, id, values);
          });
        }
      }
    },
  };

  ninjaData.push(modifier);
}

const ninja = document.querySelector("ninja-keys");
ninja.data = ninjaData;
document.getElementById("btnNinja").addEventListener("click", () => {
  ninja.open();
});