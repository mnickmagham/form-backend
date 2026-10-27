/*
 * Student Management UI ↔ Go REST API
 * Same-origin: served by nginx in production (root docker-compose.yml) or by
 * the Go server during local development. No CORS needed.
 *
 * Exactly 3 files: index.html, style.css, app.js.
 * The existing UI/markup is intentionally kept as-is; all improvements live here.
 */
(() => {
  "use strict";

  const API_BASE = resolveApiBase();
  const PAGE_SIZE = 10;
  const REQUEST_TIMEOUT_MS = 15000;
  const MAX_TOASTS = 3;

  const SERVER_CODE_MESSAGES = {
    "student already registered": "این دانشجو قبلاً ثبت شده است.",
    "student not found": "دانشجو یافت نشد.",
    "validation failed": "اطلاعات وارد شده نامعتبر است.",
    "invalid request body": "بدنه درخواست نامعتبر است.",
    "invalid student id": "شناسه دانشجو نامعتبر است.",
    "invalid page": "شماره صفحه نامعتبر است.",
    "invalid limit": "تعداد نمایش نامعتبر است.",
    "rate limit exceeded": "تعداد درخواست‌ها بیش از حد مجاز است.",
    "internal server error": "خطای داخلی سرور."
  };

  const FIELD_LABELS = {
    student_id: "شماره دانشجویی",
    name: "نام",
    last_name: "نام خانوادگی",
    major: "رشته تحصیلی",
    degree: "مقطع تحصیلی",
    phone_number: "شماره تماس"
  };

  const form = document.getElementById("student-form");
  const studentIdInput = document.getElementById("student-id");
  const fullNameInput = document.getElementById("student-name");
  const majorInput = document.getElementById("major");
  const degreeInput = document.getElementById("degree");
  const phoneInput = document.getElementById("phone");
  const submitButton = document.getElementById("submit-btn");
  const cancelEditButton = document.getElementById("cancel-edit-btn");
  const formMessage = document.getElementById("form-message");
  const searchInput = document.getElementById("search-input");
  const searchButton = document.getElementById("search-btn");
  const filterFieldSelect = document.getElementById("filter-field");
  const filterValueInput = document.getElementById("filter-value");
  const clearFilterButton = document.getElementById("clear-filter");
  const sortFieldSelect = document.getElementById("sort-field");
  const sortOrderSelect = document.getElementById("sort-order");
  const tbody = document.getElementById("students-tbody");
  const aboutLink = document.getElementById("about-link");
  const aboutModal = document.getElementById("about-modal");
  const aboutOverlay = document.getElementById("about-overlay");
  const aboutClose = document.getElementById("about-close");
  const prevButton = document.getElementById("prev-page");
  const nextButton = document.getElementById("next-page");
  const pageIndicator = document.getElementById("page-indicator");
  const tableWrap = document.getElementById("table-wrap");
  const tableLoader = document.getElementById("table-loader");
  const tableError = document.getElementById("table-error");
  const toastContainer = document.getElementById("toast-container");

  const SUBMIT_DEFAULT_HTML = submitButton.innerHTML;

  const state = {
    page: 1,
    total: 0,
    editingId: null,
    editingOriginalStudentId: "",
    currentSearch: "",
    qCache: null,
    filterField: "",
    filterValue: "",
    sortField: "id",
    sortOrder: "ASC",
    formBusy: false,
    listBusy: false,
    listError: false,
    listController: null,
    cooldownUntil: 0,
    cooldownTimer: null,
    lowQuotaWarned: false
  };

  /* ------------------------------------------------------------------ *
   * Helpers
   * ------------------------------------------------------------------ */

  function resolveApiBase() {
    const params = new URLSearchParams(window.location.search);
    const override = params.get("api");
    if (override) return override.replace(/\/+$/, "");
    if (location.protocol === "http:" || location.protocol === "https:") return location.origin;
    return "http://localhost:8080";
  }

  function isAbortError(error) {
    return Boolean(error) && (error.name === "AbortError" || error.code === 20);
  }

  function labelForField(field) {
    return FIELD_LABELS[field] || field;
  }

  function parseMissing(details) {
    const raw = details && details.missing;
    if (typeof raw !== "string") return [];
    return raw.split(",").map((item) => item.trim()).filter(Boolean);
  }

  function showMessage(element, message, kind = "") {
    element.textContent = message;
    element.className = `status-message ${kind}`.trim();
    element.hidden = !message;
  }

  function showToast(message, kind = "success") {
    const toast = document.createElement("div");
    toast.className = `toast ${kind}`.trim();
    toast.setAttribute("role", kind === "error" ? "alert" : "status");

    const text = document.createElement("span");
    text.textContent = message;
    toast.appendChild(text);

    toastContainer.appendChild(toast);
    while (toastContainer.children.length > MAX_TOASTS) {
      toastContainer.firstElementChild.remove();
    }

    setTimeout(() => {
      toast.style.transition = "opacity .25s";
      toast.style.opacity = "0";
      setTimeout(() => toast.remove(), 260);
    }, kind === "error" ? 7000 : 4200);
  }

  function cell(text) {
    const td = document.createElement("td");
    td.textContent = text == null ? "" : String(text);
    return td;
  }

  function setInvalid(input, invalid) {
    input.setAttribute("aria-invalid", invalid ? "true" : "false");
  }

  function clearInvalid() {
    [studentIdInput, fullNameInput, phoneInput].forEach((input) => setInvalid(input, false));
  }

  function totalPages() {
    return Math.max(1, Math.ceil(state.total / PAGE_SIZE));
  }

  function isLimited() {
    if (state.cooldownUntil <= Date.now()) return false;
    const left = Math.ceil((state.cooldownUntil - Date.now()) / 1000);
    showToast(`محدودیت درخواست فعال است. ${left} ثانیه دیگر تلاش کنید.`, "error");
    return true;
  }

  /* ------------------------------------------------------------------ *
   * API client
   * ------------------------------------------------------------------ */

  class ApiError extends Error {
    constructor(message, options = {}) {
      super(message);
      this.name = "ApiError";
      this.status = options.status || 0;
      this.code = options.code || "";
      this.details = options.details || null;
      this.requestId = options.requestId || "";
      this.retryAfter = options.retryAfter || 0;
    }
  }

  async function request(path, options = {}) {
    const { method = "GET", body, signal, timeout = REQUEST_TIMEOUT_MS } = options;
    const controller = new AbortController();
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, timeout);
    const forwardAbort = () => controller.abort();

    if (signal) {
      if (signal.aborted) controller.abort();
      else signal.addEventListener("abort", forwardAbort, { once: true });
    }

    const headers = { Accept: "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";

    let response;
    try {
      response = await fetch(`${API_BASE}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
        credentials: "same-origin"
      });
    } catch (error) {
      if (signal && signal.aborted) throw error;
      if (timedOut) {
        throw new ApiError("زمان پاسخ‌گویی سرور به پایان رسید. لطفاً دوباره تلاش کنید.", { code: "timeout" });
      }
      throw new ApiError(
        "ارتباط با سرور برقرار نشد. از در دسترس بودن سرور مطمئن شوید و صفحه را از همان آدرس سرویس باز کنید.",
        { code: "network" }
      );
    } finally {
      clearTimeout(timer);
      if (signal) signal.removeEventListener("abort", forwardAbort);
    }

    updateRateLimit(response.headers);

    const raw = await response.text().catch(() => "");
    let data = null;
    if (raw) {
      try {
        data = JSON.parse(raw);
      } catch {
        data = null;
      }
    }

    const requestId = response.headers.get("X-Request-ID") || "";
    const retryAfter = parseRetryAfter(response.headers);

    if (!response.ok) {
      const code = data && typeof data.error === "string" ? data.error : "";
      throw new ApiError(errorMessageFor(response.status, code, data, retryAfter), {
        status: response.status,
        code,
        details: data && data.details ? data.details : null,
        requestId,
        retryAfter
      });
    }

    return data;
  }

  function parseRetryAfter(headers) {
    const raw = headers.get("Retry-After");
    if (!raw) return 0;
    const seconds = Number.parseInt(raw, 10);
    if (Number.isFinite(seconds) && seconds >= 0) return seconds;
    const when = Date.parse(raw);
    if (!Number.isNaN(when)) return Math.max(0, Math.ceil((when - Date.now()) / 1000));
    return 0;
  }

  function errorMessageFor(status, code, data, retryAfter) {
    if (status === 429 || code === "rate limit exceeded") {
      return `تعداد درخواست‌ها بیش از حد مجاز است. لطفاً ${retryAfter > 0 ? retryAfter : 60} ثانیه دیگر دوباره تلاش کنید.`;
    }
    if (status === 409 || code === "student already registered") {
      return "دانشجویی با این شماره دانشجویی قبلاً ثبت شده است.";
    }
    if (status === 404 || code === "student not found") {
      return "دانشجویی با این مشخصات یافت نشد. ممکن است قبلاً حذف شده باشد.";
    }
    if (status === 400) {
      if (code === "validation failed") {
        const missing = parseMissing(data && data.details);
        if (missing.length) {
          return `فیلدهای ضروری تکمیل نشده‌اند: ${missing.map(labelForField).join("، ")}.`;
        }
        return "اطلاعات وارد شده نامعتبر است.";
      }
      if (code === "invalid request body") return "داده‌های ارسالی نامعتبر است. لطفاً صفحه را دوباره بارگذاری کنید.";
      if (code === "invalid student id") return "شناسه دانشجو نامعتبر است.";
      if (code === "invalid page" || code === "invalid limit") return "پارامترهای صفحه‌بندی نامعتبر است.";
      return "درخواست نامعتبر است. ورودی‌ها را بررسی کنید.";
    }
    if (status === 500) return "خطای داخلی سرور رخ داد. لطفاً دوباره تلاش کنید.";
    if (status === 503) return "سرویس در حال حاضر در دسترس نیست. لطفاً چند لحظه دیگر تلاش کنید.";
    if (code) return `خطای سرور: ${SERVER_CODE_MESSAGES[code] || code}`;
    return `درخواست با خطا مواجه شد (HTTP ${status}).`;
  }

  function formatError(error) {
    return error.requestId ? `${error.message} کد پیگیری: ${error.requestId}` : error.message;
  }

  function toApiError(error) {
    return error instanceof ApiError ? error : new ApiError("خطای ناشناخته رخ داد.");
  }

  /* ------------------------------------------------------------------ *
   * Rate limit handling (server sends X-RateLimit-* and Retry-After)
   * ------------------------------------------------------------------ */

  function updateRateLimit(headers) {
    const remaining = Number.parseInt(headers.get("X-RateLimit-Remaining") || "", 10);
    if (!Number.isFinite(remaining)) return;

    const title = `درخواست باقی‌مانده: ${remaining}`;
    submitButton.title = title;
    searchButton.title = title;

    if (remaining > 5) {
      state.lowQuotaWarned = false;
    } else if (remaining > 0 && !state.lowQuotaWarned) {
      state.lowQuotaWarned = true;
      showToast(`سهمیه درخواست‌ها رو به پایان است (${remaining} باقی‌مانده).`, "error");
    }
  }

  function startCooldown(seconds, target) {
    const until = Date.now() + Math.max(1, seconds) * 1000;
    if (until <= state.cooldownUntil) return;

    state.cooldownUntil = until;
    clearInterval(state.cooldownTimer);

    const tick = () => {
      const left = Math.max(0, Math.ceil((state.cooldownUntil - Date.now()) / 1000));
      if (left <= 0) {
        clearInterval(state.cooldownTimer);
        state.cooldownTimer = null;
        state.cooldownUntil = 0;

        if (target === tableError) {
          tableError.textContent = "";
          tableError.hidden = true;
          // The list failed because of the rate limit: retry once the window
          // resets so the user is not stuck on an unusable table.
          if (state.listError) {
            loadStudents();
            return;
          }
        } else if (target) {
          showMessage(target, "");
        }

        refreshDisabledStates();
        return;
      }

      const text = `محدودیت درخواست فعال است. ${left} ثانیه دیگر دوباره تلاش کنید.`;
      if (target === tableError) {
        tableError.textContent = text;
        tableError.hidden = false;
      } else if (target) {
        showMessage(target, text, "error");
      }
    };

    tick();
    state.cooldownTimer = setInterval(tick, 1000);
    refreshDisabledStates();
  }

  function refreshDisabledStates() {
    const limited = state.cooldownUntil > Date.now();
    const pages = totalPages();

    submitButton.disabled = state.formBusy || limited;
    cancelEditButton.disabled = state.formBusy;
    searchButton.disabled = limited;
    prevButton.disabled = state.listBusy || state.listError || limited || state.page <= 1;
    nextButton.disabled = state.listBusy || state.listError || limited || state.page >= pages;
  }

  /* ------------------------------------------------------------------ *
   * Form
   * ------------------------------------------------------------------ */

  function splitFullName(fullName) {
    const parts = fullName.trim().split(/\s+/).filter(Boolean);
    return { name: parts.shift() || "", last_name: parts.join(" ") };
  }

  function buildPayload() {
    const { name, last_name } = splitFullName(fullNameInput.value);
    return {
      student_id: studentIdInput.value.trim(),
      name,
      last_name,
      major: majorInput.value,
      degree: degreeInput.value,
      phone_number: phoneInput.value.trim()
    };
  }

  function validatePayload(payload) {
    clearInvalid();
    let message = "";
    let focusTarget = null;

    if (!payload.student_id) {
      message = "شماره دانشجویی را وارد کنید.";
      focusTarget = studentIdInput;
      setInvalid(studentIdInput, true);
    } else if (payload.student_id.length > 20) {
      message = "شماره دانشجویی نباید بیشتر از 20 کاراکتر باشد.";
      focusTarget = studentIdInput;
      setInvalid(studentIdInput, true);
    } else if (!payload.name || !payload.last_name) {
      message = "نام و نام خانوادگی را کامل وارد کنید (مثال: علی محمدی).";
      focusTarget = fullNameInput;
      setInvalid(fullNameInput, true);
    } else if (payload.name.length > 100 || payload.last_name.length > 100) {
      message = "نام و نام خانوادگی نباید بیشتر از 100 کاراکتر باشد.";
      focusTarget = fullNameInput;
      setInvalid(fullNameInput, true);
    } else if (payload.phone_number.length > 30) {
      message = "شماره تماس نباید بیشتر از 30 کاراکتر باشد.";
      focusTarget = phoneInput;
      setInvalid(phoneInput, true);
    }

    if (message) {
      showMessage(formMessage, message, "error");
      if (focusTarget) focusTarget.focus();
      return false;
    }

    showMessage(formMessage, "");
    return true;
  }

  function setSelectValue(select, value) {
    const next = value || "";
    if (next && !Array.from(select.options).some((option) => option.value === next)) {
      const option = document.createElement("option");
      option.value = next;
      option.textContent = next;
      option.dataset.dynamic = "true";
      select.appendChild(option);
    }
    select.value = next;
  }

  function clearDynamicOptions(select) {
    select.querySelectorAll('option[data-dynamic="true"]').forEach((option) => option.remove());
  }

  function applyStudentToForm(student) {
    studentIdInput.value = student.student_id || "";
    fullNameInput.value = [student.name, student.last_name].filter(Boolean).join(" ");
    setSelectValue(majorInput, student.major);
    setSelectValue(degreeInput, student.degree);
    phoneInput.value = student.phone_number || "";
    clearInvalid();
  }

  function clearForm() {
    form.reset();
    clearDynamicOptions(majorInput);
    clearDynamicOptions(degreeInput);
    state.editingId = null;
    state.editingOriginalStudentId = "";
    submitButton.innerHTML = SUBMIT_DEFAULT_HTML;
    cancelEditButton.hidden = true;
    clearInvalid();
    showMessage(formMessage, "");
    refreshDisabledStates();
  }

  function setFormBusy(busy) {
    state.formBusy = busy;
    submitButton.setAttribute("aria-busy", busy ? "true" : "false");
    refreshDisabledStates();
  }

  async function studentIdIsTaken(studentId, ignoredId) {
    const params = new URLSearchParams({
      student_id: studentId,
      page: "1",
      limit: "5"
    });
    const data = await request(`/students?${params.toString()}`);
    const students = Array.isArray(data && data.students) ? data.students : [];
    return students.some((student) => student.student_id === studentId && student.id !== ignoredId);
  }

  async function handleSubmit(event) {
    event.preventDefault();
    if (state.formBusy || isLimited()) return;

    const payload = buildPayload();
    if (!validatePayload(payload)) return;

    setFormBusy(true);

    try {
      if (state.editingId !== null) {
        if (payload.student_id !== state.editingOriginalStudentId) {
          const taken = await studentIdIsTaken(payload.student_id, state.editingId);
          if (taken) {
            setInvalid(studentIdInput, true);
            showMessage(formMessage, "این شماره دانشجویی برای دانشجوی دیگری ثبت شده است.", "error");
            showToast("شماره دانشجویی تکراری است.", "error");
            studentIdInput.focus();
            return;
          }
        }

        await request(`/students/${encodeURIComponent(state.editingId)}`, {
          method: "PUT",
          body: payload
        });
        showToast("اطلاعات دانشجو با موفقیت ویرایش شد.", "success");
        clearForm();
        state.qCache = null;
        await loadStudents();
      } else {
        const created = await request("/students", { method: "POST", body: payload });
        const createdId = created && created.student_id ? created.student_id : payload.student_id;
        showToast(`دانشجو ${createdId} با موفقیت ثبت شد.`, "success");
        clearForm();

        state.currentSearch = "";
        searchInput.value = "";
        state.page = 1;
        state.qCache = null;
        syncUrl();
        await loadStudents();
      }
    } catch (error) {
      handleSubmitError(error);
    } finally {
      setFormBusy(false);
    }
  }

  function handleSubmitError(error) {
    if (isAbortError(error)) return;
    const apiError = toApiError(error);
    let message = formatError(apiError);

    if (apiError.status === 409) {
      message = "این دانشجو قبلاً ثبت شده است. شماره دانشجویی تکراری است.";
    } else if (apiError.status === 400 && apiError.code === "validation failed") {
      const missing = parseMissing(apiError.details);
      if (missing.length) {
        message = `فیلدهای ضروری تکمیل نشده‌اند: ${missing.map(labelForField).join("، ")}.`;
      }
    } else if (apiError.status === 404) {
      clearForm();
      state.qCache = null;
      loadStudents();
    }

    showMessage(formMessage, message, "error");
    showToast(message, "error");

    if (apiError.status === 429) {
      startCooldown(apiError.retryAfter || 60, formMessage);
    }
  }

  /* ------------------------------------------------------------------ *
   * List / search / pagination
   * ------------------------------------------------------------------ */

  function filterStudents(students) {
    if (!state.filterField || !state.filterValue.trim()) return students;
    const value = state.filterValue.trim().toLowerCase();
    const field = state.filterField;
    return students.filter((student) => {
      const raw = student == null ? "" : student[field];
      return String(raw == null ? "" : raw).toLowerCase().includes(value);
    });
  }

  function sortStudents(students) {
    if (!state.sortField) return students;
    const dir = state.sortOrder === "DESC" ? -1 : 1;
    return [...students].sort((a, b) => {
      const av = a == null ? "" : a[state.sortField];
      const bv = b == null ? "" : b[state.sortField];
      if (av < bv) return -dir;
      if (av > bv) return dir;
      return 0;
    });
  }

  function sliceSearchResults(students) {
    let working = filterStudents(students);
    working = sortStudents(working);
    const pages = Math.max(1, Math.ceil(working.length / PAGE_SIZE));
    const page = Math.min(Math.max(1, state.page), pages);
    const start = (page - 1) * PAGE_SIZE;
    return {
      students: working.slice(start, start + PAGE_SIZE),
      total: working.length,
      page
    };
  }

  async function fetchStudents(signal) {
    if (state.currentSearch) {
      if (state.qCache && state.qCache.term === state.currentSearch) {
        return sliceSearchResults(state.qCache.students);
      }
      const data = await request(`/students?q=${encodeURIComponent(state.currentSearch)}`, { signal });
      const students = Array.isArray(data && data.students) ? data.students : [];
      state.qCache = { term: state.currentSearch, students };
      return sliceSearchResults(students);
    }

    const params = new URLSearchParams({
      page: String(state.page),
      limit: String(PAGE_SIZE),
      sortBy: state.sortField,
      sortOrder: state.sortOrder
    });
    if (state.filterField && state.filterValue.trim()) {
      params.set(state.filterField, state.filterValue.trim());
    }
    const data = await request(`/students?${params.toString()}`, { signal });
    const students = Array.isArray(data && data.students) ? data.students : [];
    return {
      students,
      total: Number(data && data.total != null ? data.total : students.length),
      page: Number(data && data.page != null ? data.page : state.page)
    };
  }

  async function loadStudents() {
    if (state.listController) state.listController.abort();
    const controller = new AbortController();
    state.listController = controller;
    state.listError = false;
    setListBusy(true);
    tableError.hidden = true;
    tableError.textContent = "";

    try {
      let result = await fetchStudents(controller.signal);
      if (controller.signal.aborted) return;

      const pages = Math.max(1, Math.ceil(result.total / PAGE_SIZE));
      if (result.total > 0 && state.page > pages) {
        state.page = pages;
        result = await fetchStudents(controller.signal);
      } else if (result.total === 0 && state.page > 1) {
        state.page = 1;
      }
      if (controller.signal.aborted) return;

      state.total = result.total;
      state.page = result.page || state.page;
      renderStudents(result.students);
      renderPagination();
      syncUrl();
    } catch (error) {
      if (isAbortError(error) || controller.signal.aborted) return;
      handleListError(error);
    } finally {
      if (state.listController === controller) {
        state.listController = null;
        setListBusy(false);
      }
    }
  }

  function setListBusy(busy) {
    state.listBusy = busy;
    tableWrap.classList.toggle("loading", busy);
    tableLoader.style.display = busy ? "flex" : "none";
    tableWrap.setAttribute("aria-busy", busy ? "true" : "false");
    refreshDisabledStates();
  }

  function renderPagination() {
    pageIndicator.textContent = `${state.page} از ${totalPages()}`;
    refreshDisabledStates();
  }

  function handleListError(error) {
    const apiError = toApiError(error);
    const message = formatError(apiError);

    state.listError = true;
    state.total = 0;
    tbody.replaceChildren(messageRow("دریافت اطلاعات ممکن نشد."));
    tableError.textContent = message;
    tableError.hidden = false;
    pageIndicator.textContent = "—";
    showToast(message, "error");

    if (apiError.status === 429) {
      startCooldown(apiError.retryAfter || 60, tableError);
    }
    refreshDisabledStates();
  }

  function messageRow(message) {
    const tr = document.createElement("tr");
    const td = cell(message);
    td.colSpan = 7;
    tr.appendChild(td);
    return tr;
  }

  function setRowBusy(tr, busy) {
    if (!tr) return;
    tr.querySelectorAll("button").forEach((button) => {
      button.disabled = busy;
    });
  }

  function renderStudents(students) {
    tbody.replaceChildren();

    if (!students.length) {
      tbody.appendChild(messageRow(state.currentSearch ? "دانشجویی یافت نشد." : "اطلاعاتی برای نمایش وجود ندارد."));
      return;
    }

    students.forEach((student, index) => {
      const tr = document.createElement("tr");

      tr.appendChild(cell((state.page - 1) * PAGE_SIZE + index + 1));
      tr.appendChild(cell(student.student_id));
      tr.appendChild(cell([student.name, student.last_name].filter(Boolean).join(" ")));
      tr.appendChild(cell(student.major));
      tr.appendChild(cell(student.degree));
      tr.appendChild(cell(student.phone_number));

      const actions = document.createElement("td");
      actions.className = "actions";

      const edit = document.createElement("button");
      edit.type = "button";
      edit.className = "edit";
      edit.setAttribute("aria-label", "ویرایش");
      edit.title = "ویرایش";
      edit.textContent = "✎";
      edit.addEventListener("click", () => editStudent(student, tr));

      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "delete";
      remove.setAttribute("aria-label", "حذف");
      remove.title = "حذف";
      remove.textContent = "▣";
      remove.addEventListener("click", () => deleteStudent(student, tr));

      actions.append(edit, remove);
      tr.appendChild(actions);
      tbody.appendChild(tr);
    });
  }

  async function editStudent(student, tr) {
    if (isLimited()) return;
    setRowBusy(tr, true);

    try {
      // GET /students/{id}: edit always starts from fresh server data.
      const fresh = await request(`/students/${encodeURIComponent(student.id)}`);
      applyStudentToForm(fresh);
      state.editingId = fresh.id;
      state.editingOriginalStudentId = fresh.student_id || "";
      submitButton.innerHTML = "ذخیره تغییرات";
      cancelEditButton.hidden = false;
      showMessage(formMessage, `در حال ویرایش اطلاعات دانشجو با شماره ${fresh.student_id}`);
      form.scrollIntoView({ behavior: "smooth", block: "start" });
    } catch (error) {
      handleActionError(error);
    } finally {
      if (tr.isConnected) setRowBusy(tr, false);
    }
  }

  async function deleteStudent(student, tr) {
    if (isLimited()) return;
    if (!window.confirm(`اطلاعات دانشجو ${student.student_id} حذف شود؟`)) return;

    setRowBusy(tr, true);

    try {
      await request(`/students/${encodeURIComponent(student.id)}`, { method: "DELETE" });
      showToast("اطلاعات دانشجو حذف شد.", "success");

      if (state.editingId === student.id) clearForm();
      state.qCache = null;

      if ((state.page - 1) * PAGE_SIZE >= state.total - 1 && state.page > 1) {
        state.page -= 1;
      }
      await loadStudents();
    } catch (error) {
      handleActionError(error);
    } finally {
      if (tr.isConnected) setRowBusy(tr, false);
    }
  }

  function handleActionError(error) {
    if (isAbortError(error)) return;
    const apiError = toApiError(error);
    const message = formatError(apiError);

    showToast(message, "error");

    if (apiError.status === 429) {
      startCooldown(apiError.retryAfter || 60, tableError);
    }
    if (apiError.status === 404) {
      state.qCache = null;
      loadStudents();
    }
  }

  /* ------------------------------------------------------------------ *
   * Search / pagination events
   * ------------------------------------------------------------------ */

  function runSearch() {
    if (isLimited()) return;
    const term = searchInput.value.trim();

    if (!term) {
      clearSearch();
      return;
    }

    state.currentSearch = term;
    state.page = 1;
    state.qCache = null;
    state.filterField = "";
    state.filterValue = "";
    syncUrl();
    loadStudents();
  }

  function clearSearch() {
    if (isLimited()) return;
    state.currentSearch = "";
    state.page = 1;
    state.qCache = null;
    searchInput.value = "";
    syncUrl();
    loadStudents();
  }

  function applyFilter() {
    if (isLimited()) return;
    state.filterField = filterFieldSelect.value;
    state.filterValue = filterValueInput.value.trim();
    state.page = 1;
    syncUrl();
    loadStudents();
  }

  function clearFilter() {
    if (isLimited()) return;
    state.filterField = "";
    state.filterValue = "";
    filterValueInput.value = "";
    state.currentSearch = "";
    searchInput.value = "";
    state.qCache = null;
    state.page = 1;
    syncUrl();
    loadStudents();
  }

  function applySort() {
    if (isLimited()) return;
    state.sortField = sortFieldSelect.value;
    state.sortOrder = sortOrderSelect.value;
    state.page = 1;
    syncUrl();
    loadStudents();
  }

  function goToPage(page) {
    if (isLimited()) return;
    const target = Math.min(Math.max(1, page), totalPages());
    if (target === state.page) return;
    state.page = target;
    syncUrl();
    loadStudents();
  }

  /* ------------------------------------------------------------------ *
   * URL state (q + page) so refreshes/back/forward keep the view
   * ------------------------------------------------------------------ */

  function readInitialState() {
    const params = new URLSearchParams(window.location.search);
    state.currentSearch = (params.get("q") || "").trim();
    searchInput.value = state.currentSearch;

    state.filterField = params.get("filter_field") || "";
    state.filterValue = params.get("filter_value") || "";
    if (filterFieldSelect) filterFieldSelect.value = state.filterField || "";
    if (filterValueInput) filterValueInput.value = state.filterValue;

    state.sortField = params.get("sort_by") || "id";
    state.sortOrder = params.get("sort_order") || "ASC";
    if (sortFieldSelect) sortFieldSelect.value = state.sortField;
    if (sortOrderSelect) sortOrderSelect.value = state.sortOrder;

    const page = Number.parseInt(params.get("page") || "", 10);
    state.page = Number.isFinite(page) && page > 0 ? page : 1;
  }

  function syncUrl() {
    if (location.protocol === "file:") return;

    const params = new URLSearchParams(window.location.search);
    const apiOverride = params.get("api");
    params.delete("q");
    params.delete("page");
    params.delete("filter_field");
    params.delete("filter_value");
    params.delete("sort_by");
    params.delete("sort_order");
    if (apiOverride) params.set("api", apiOverride);

    if (state.currentSearch) params.set("q", state.currentSearch);
    if (state.filterField) params.set("filter_field", state.filterField);
    if (state.filterValue) params.set("filter_value", state.filterValue);
    if (state.sortField) params.set("sort_by", state.sortField);
    if (state.sortOrder) params.set("sort_order", state.sortOrder);
    if (state.page > 1) params.set("page", String(state.page));

    const query = params.toString();
    try {
      history.replaceState(null, "", `${location.pathname}${query ? `?${query}` : ""}${location.hash}`);
    } catch {
      /* ignore (older browsers / sandboxed contexts) */
    }
  }

  /* ------------------------------------------------------------------ *
   * Events / init
   * ------------------------------------------------------------------ */

  form.addEventListener("submit", handleSubmit);
  cancelEditButton.addEventListener("click", clearForm);

  [studentIdInput, fullNameInput, phoneInput].forEach((input) => {
    input.addEventListener("input", () => setInvalid(input, false));
  });

  searchButton.addEventListener("click", runSearch);
  searchInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      runSearch();
    }
  });
  searchInput.addEventListener("search", () => {
    if (!searchInput.value.trim()) clearSearch();
  });

  clearFilterButton.addEventListener("click", clearFilter);
  filterFieldSelect.addEventListener("change", applyFilter);
  filterValueInput.addEventListener("input", applyFilter);
  sortFieldSelect.addEventListener("change", applySort);
  sortOrderSelect.addEventListener("change", applySort);

  function openAbout() {
    aboutModal.hidden = false;
  }

  function closeAbout() {
    aboutModal.hidden = true;
  }

  aboutLink.addEventListener("click", (event) => {
    event.preventDefault();
    openAbout();
  });
  aboutClose.addEventListener("click", closeAbout);
  aboutOverlay.addEventListener("click", closeAbout);

  prevButton.addEventListener("click", () => goToPage(state.page - 1));
  nextButton.addEventListener("click", () => goToPage(state.page + 1));

  formMessage.setAttribute("aria-live", "polite");
  tableError.setAttribute("role", "alert");

  window.addEventListener("online", () => showToast("اتصال شبکه برقرار شد.", "success"));
  window.addEventListener("offline", () => showToast("اتصال شبکه قطع شد.", "error"));
  window.addEventListener("popstate", () => {
    readInitialState();
    state.qCache = null;
    loadStudents();
  });

  readInitialState();
  refreshDisabledStates();
  loadStudents();
})();
