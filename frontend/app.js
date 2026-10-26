/*
 * Student Management UI ↔ Go REST API
 * Recommended: place index.html, style.css and app.js in server/ui
 * and open http://localhost:8080/ so the UI and API share one origin.
 */
(() => {
  "use strict";

  const API_BASE = "http://localhost:8080"; // Go backend address. Change this if your backend uses another host/port.
  const PAGE_SIZE = 10;

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
  const tbody = document.getElementById("students-tbody");
  const prevButton = document.getElementById("prev-page");
  const nextButton = document.getElementById("next-page");
  const pageIndicator = document.getElementById("page-indicator");

  let page = 1;
  let total = 0;
  let editingDatabaseId = null;
  let currentSearch = "";

  function showMessage(element, message, kind = "") {
    element.textContent = message;
    element.className = `status-message ${kind}`.trim();
  }

  async function request(path, options = {}) {
    let response;
    try {
      response = await fetch(`${API_BASE}${path}`, {
        ...options,
        headers: {
          ...(options.body ? { "Content-Type": "application/json" } : {}),
          ...(options.headers || {})
        }
      });
    } catch (error) {
      throw new Error("ارتباط با سرور برقرار نشد. مطمئن شوید بک‌اند اجراست و صفحه را از http://localhost:8080 باز کرده‌اید.");
    }

    const raw = await response.text();
    let data = null;
    if (raw) {
      try { data = JSON.parse(raw); } catch { data = null; }
    }
    if (!response.ok) {
      const detail = data?.details?.missing
        ? `فیلدهای ضروری: ${data.details.missing}`
        : (data?.error || data?.message || `درخواست به آدرس ${path} پاسخ مناسب نداد (HTTP ${response.status}). آدرس بک‌اند را بررسی کنید.`);
      throw new Error(detail);
    }
    return data;
  }

  function splitFullName(fullName) {
    const parts = fullName.trim().split(/\s+/).filter(Boolean);
    return { name: parts.shift() || "", last_name: parts.join(" ") };
  }

  function getStudentPayload() {
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

  function setFormFromStudent(student) {
    studentIdInput.value = student.student_id || "";
    fullNameInput.value = [student.name, student.last_name].filter(Boolean).join(" ");
    majorInput.value = student.major || "";
    degreeInput.value = student.degree || "";
    phoneInput.value = student.phone_number || "";
  }

  function clearForm() {
    form.reset();
    editingDatabaseId = null;
    submitButton.innerHTML = 'ثبت اطلاعات <span>＋</span>';
    cancelEditButton.hidden = true;
    showMessage(formMessage, "");
  }

  function cell(text) {
    const td = document.createElement("td");
    td.textContent = text == null ? "" : String(text);
    return td;
  }

  function renderStudents(students) {
    tbody.replaceChildren();
    if (!students.length) {
      const tr = document.createElement("tr");
      const td = cell("دانشجویی یافت نشد.");
      td.colSpan = 7;
      tr.appendChild(td);
      tbody.appendChild(tr);
      return;
    }

    students.forEach((student, index) => {
      const tr = document.createElement("tr");
      tr.appendChild(cell((page - 1) * PAGE_SIZE + index + 1));
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
      edit.addEventListener("click", () => {
        editingDatabaseId = student.id;
        setFormFromStudent(student);
        submitButton.innerHTML = "ذخیره تغییرات";
        cancelEditButton.hidden = false;
        showMessage(formMessage, `در حال ویرایش اطلاعات دانشجو با شماره ${student.student_id}`);
        form.scrollIntoView({ behavior: "smooth", block: "start" });
      });

      const del = document.createElement("button");
      del.type = "button";
      del.className = "delete";
      del.setAttribute("aria-label", "حذف");
      del.title = "حذف";
      del.textContent = "▣";
      del.addEventListener("click", async () => {
        if (!window.confirm(`اطلاعات دانشجو ${student.student_id} حذف شود؟`)) return;
        try {
          await request(`/students/${encodeURIComponent(student.id)}`, { method: "DELETE" });
          showMessage(formMessage, "اطلاعات دانشجو حذف شد.", "success");
          if ((page - 1) * PAGE_SIZE >= total - 1 && page > 1) page--;
          await loadStudents();
        } catch (error) {
          showMessage(formMessage, error.message, "error");
        }
      });

      actions.append(edit, del);
      tr.appendChild(actions);
      tbody.appendChild(tr);
    });
  }

  async function loadStudents() {
    tbody.innerHTML = '<tr><td colspan="7">در حال بارگذاری اطلاعات...</td></tr>';
    try {
      const params = new URLSearchParams({ page: String(page), limit: String(PAGE_SIZE) });
      if (currentSearch) params.set("q", currentSearch);
      const data = await request(`/students?${params.toString()}`);
      const students = Array.isArray(data) ? data : (data?.students || []);
      total = Number(data?.total ?? students.length);
      renderStudents(students);
      const lastPage = Math.max(1, Math.ceil(total / PAGE_SIZE));
      pageIndicator.textContent = `${page} از ${lastPage}`;
      prevButton.disabled = page <= 1;
      nextButton.disabled = page >= lastPage || Boolean(currentSearch);
    } catch (error) {
      tbody.replaceChildren();
      const tr = document.createElement("tr");
      const td = cell(error.message);
      td.colSpan = 7;
      tr.appendChild(td);
      tbody.appendChild(tr);
      pageIndicator.textContent = "—";
      prevButton.disabled = true;
      nextButton.disabled = true;
    }
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const payload = getStudentPayload();

    if (!payload.student_id || !payload.name || !payload.last_name) {
      showMessage(formMessage, "شماره دانشجویی و نام و نام خانوادگی را وارد کنید (مثال: علی محمدی؛ نام باید شامل نام و نام خانوادگی باشد).", "error");
      return;
    }

    submitButton.disabled = true;
    try {
      if (editingDatabaseId !== null) {
        await request(`/students/${encodeURIComponent(editingDatabaseId)}`, {
          method: "PUT",
          body: JSON.stringify(payload)
        });
        showMessage(formMessage, "اطلاعات دانشجو با موفقیت ویرایش شد.", "success");
      } else {
        await request("/students", {
          method: "POST",
          body: JSON.stringify(payload)
        });
        showMessage(formMessage, "اطلاعات دانشجو با موفقیت ثبت شد.", "success");
      }
      clearForm();
      currentSearch = "";
      searchInput.value = "";
      page = 1;
      await loadStudents();
      showMessage(formMessage, editingDatabaseId === null ? "عملیات با موفقیت انجام شد." : "عملیات با موفقیت انجام شد.", "success");
    } catch (error) {
      showMessage(formMessage, error.message, "error");
    } finally {
      submitButton.disabled = false;
    }
  });

  function runSearch() {
    currentSearch = searchInput.value.trim();
    page = 1;
    loadStudents();
  }

  searchButton.addEventListener("click", runSearch);
  searchInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") runSearch();
  });
  searchInput.addEventListener("search", runSearch);
  prevButton.addEventListener("click", () => {
    if (page > 1) { page--; loadStudents(); }
  });
  nextButton.addEventListener("click", () => {
    if (page * PAGE_SIZE < total && !currentSearch) { page++; loadStudents(); }
  });
  cancelEditButton.addEventListener("click", clearForm);

  loadStudents();
})();
