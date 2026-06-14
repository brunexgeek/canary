(() => {
    const PAGE_URL = (location.origin + location.pathname).replace(/\/$/, '');
    let CONFIG = {};
    const ROOT_ELEMENT = findRootElement();

    const newForm = document.getElementById("new-comment-form");
    const usernameEl = document.getElementById("username");
    const textEl = document.getElementById("text");

    // cache so we don't fully refetch on every reply
    let commentCache = [];

    async function sha256(str) {
        const data = new TextEncoder().encode(str);
        const hashBuffer = await crypto.subtle.digest('SHA-256', data);

        return Array.from(new Uint8Array(hashBuffer))
            .map(b => b.toString(16).padStart(2, '0'))
            .join('');
    }

    function findRootElement() {
        const element = document.querySelector('canary-comments');
        if (!element) {
            console.warn("[canary] Unable to find a 'canary-comments' element to display this page's comments");
            return null;
        }

        for (const attr of element.attributes) {
            CONFIG[attr.name] = attr.value;
        }
        CONFIG['api-url'] = CONFIG['api-url'].replace(/\/$/, '');
        return element;
    }

    function base64UrlEncode(str) {
        const bytes = new TextEncoder().encode(str);

        let base64 = btoa(String.fromCharCode(...bytes));

        return base64
            .replace(/\+/g, '-')
            .replace(/\//g, '_')
            .replace(/=+$/, '');
    }

    async function fetchComments() {
        const site = base64UrlEncode(document.location.origin);
        const page = base64UrlEncode(document.location.pathname);
        const res = await fetch(`${CONFIG['api-url']}/${site}/${page}`);
        commentCache = await res.json();
        render();
    }

    function render() {
        requestAnimationFrame( () => {
            ROOT_ELEMENT.innerHTML = "";
            commentCache.forEach(c => ROOT_ELEMENT.appendChild(renderComment(c)));
        });
    }

    function renderComment(comment) {
        const wrap = document.createElement("div");
        wrap.classList.add("comment");
        const card = document.createElement("div");
        card.classList.add("card");
        wrap.dataset.id = comment.id;

        const header = document.createElement("div");

        const collapseBtn = document.createElement("button");
        collapseBtn.textContent = "-";
        collapseBtn.onclick = () => {
            const body = wrap.querySelector(".body");
            const replies = wrap.querySelector(".replies");
            const hidden = body.style.display === "none";
            body.style.display = hidden ? "" : "none";
            if (replies) replies.style.display = hidden ? "" : "none";
            collapseBtn.textContent = hidden ? "-" : "+";
        };

        header.innerHTML = `
      <strong>${escapeHtml(comment.username)}</strong>
      <small>#${comment.id}</small>
      <small>${new Date(comment.created_at).toLocaleString()}</small>
    `;

        header.prepend(collapseBtn);

        const body = document.createElement("div");
        body.className = "body";
        body.innerHTML = renderMarkdown(comment.text);

        const actions = document.createElement("div");

        const replyBtn = document.createElement("button");
        replyBtn.textContent = "Reply";

        const replyBox = document.createElement("div");

        replyBtn.onclick = () => {
            if (replyBox.childNodes.length) {
                replyBox.innerHTML = "";
                return;
            }

            replyBox.appendChild(createReplyForm(comment.id));
        };

        actions.appendChild(replyBtn);

        card.appendChild(header);
        card.appendChild(body);
        card.appendChild(actions);
        wrap.appendChild(card);
        wrap.appendChild(replyBox);

        // replies
        if (comment.replies?.length) {
            const replies = document.createElement("div");
            replies.className = "replies";
            replies.style.marginLeft = "20px";

            comment.replies.forEach(r => {
                replies.appendChild(renderComment(r));
            });

            wrap.appendChild(replies);
        }

        return wrap;
    }

    // -------------------------
    // Reply form
    // -------------------------
    function createReplyForm(parentId) {
        const form = document.createElement("form");

        form.innerHTML = `
      <textarea name="text" placeholder="reply..." required></textarea>
      <br>
      <button>Send reply</button>
      <button>Cancel</button>
    `;

        form.addEventListener("submit", async (e) => {
            e.preventDefault();

            const text = form.text.value;

            //const tempComment = optimisticAdd(text, parentId);

            try {
                await postComment(text, parentId);
                await fetchComments();
            } catch (err) {
                console.error(err);
                //alert("Failed to post");
                //removeOptimistic(tempComment.id);
            }
        });

        return form;
    }

    // -------------------------
    // New comment form
    // -------------------------
    newForm.addEventListener("submit", async (e) => {
        e.preventDefault();

        const text = textEl.value;

        //const temp = optimisticAdd(text, null);

        textEl.value = "";

        try {
            await postComment(text, null);
            await fetchComments();
        } catch (err) {
            console.error(err);
            //alert("Failed");
            //removeOptimistic(temp.id);
        }
    });

    // -------------------------
    // Optimistic UI
    // -------------------------
    function optimisticAdd(text, parentId) {
        const temp = {
            text,
            parent_id: parentId,
            replies: []
        };

        insertIntoTree(commentCache, temp);
        render();

        return temp;
    }

    function removeOptimistic(id) {
        commentCache = removeFromTree(commentCache, id);
        render();
    }

    function insertIntoTree(tree, comment) {
        if (!comment.parent_id) {
            tree.unshift(comment);
            return;
        }

        const parent = findComment(tree, comment.parent_id);
        if (parent) parent.replies.push(comment);
    }

    function removeFromTree(tree, id) {
        return tree
            .filter(c => c.id !== id)
            .map(c => ({
                ...c,
                replies: c.replies ? removeFromTree(c.replies, id) : []
            }));
    }

    function findComment(tree, id) {
        for (const c of tree) {
            if (c.id == id) return c;
            if (c.replies) {
                const found = findComment(c.replies, id);
                if (found) return found;
            }
        }
        return null;
    }

    // -------------------------
    // API
    // -------------------------
    async function postComment(text, parent_id) {
        await fetch(`${API_BASE}/comment`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                url: PAGE_URL,
                text,
                parent_id
            })
        });
    }

    // -------------------------
    // Markdown (VERY minimal safe subset)
    // -------------------------
    function renderMarkdown(text) {
        return escapeHtml(text)
            .replace(/\*\*(.+?)\*\*/g, "<b>$1</b>")
            .replace(/\*(.+?)\*/g, "<i>$1</i>")
            .replace(/`(.+?)`/g, "<code>$1</code>");
    }

    // -------------------------
    // Cookie helper
    // -------------------------
    function getCookie(name) {
        return document.cookie
            .split("; ")
            .find(row => row.startsWith(name + "="))
            ?.split("=")[1];
    }

    function escapeHtml(str) {
        return String(str)
            .replaceAll("&", "&amp;")
            .replaceAll("<", "&lt;")
            .replaceAll(">", "&gt;");
    }

    fetchComments();
})();
