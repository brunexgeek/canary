(() => {
    const PAGE_URL = (location.origin + location.pathname).replace(/\/$/, '');
    let CONFIG = {};
    const ROOT_ELEMENT = findRootElement();

    const newForm = document.getElementById("new-comment-form");
    const usernameEl = document.getElementById("username");
    const textEl = document.getElementById("text");

    // cache so we don't fully refetch on every reply
    let commentCache = [];

    console.info(document.currentScript.src);

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

    async function fetchComments() {
        const url = encodeURIComponent(document.location.href);
        const res = await fetch(`${CONFIG['api-url']}/comments?url=${url}`);
        commentCache = await res.json();
        render();
    }

    function render() {
        requestAnimationFrame( () => {
            ROOT_ELEMENT.innerHTML = "";
            commentCache.forEach(c => ROOT_ELEMENT.appendChild(renderComment(c)));
            // event listener for 'Reply' and 'Cancel' buttons
            ROOT_ELEMENT.querySelectorAll("[data-toggle='reply-form']").forEach( element => {
                element.addEventListener("click", event => {
                    const replyForm = document.getElementById(event.currentTarget.getAttribute("data-target"));
                    replyForm.classList.toggle("d-none");
                })
            });
            // event listener for 'Submit' buttons
            setSubmitListeners();
        });
    }

    function renderComment(comment) {
        const info = document.createElement("div");
        info.classList.add("comment-info");
        info.innerHTML = `
            <a href="#" class="comment-author">${comment.username}</a>
            <p class="m-0">
                4 days ago
            </p>
        `;

        const icon = document.createElement("img");
        icon.setAttribute("src", `${CONFIG['api-url']}/avatar?user=${encodeURIComponent(comment.username)}`);

        const heading = document.createElement("div");
        heading.classList.add("comment-heading");
        heading.appendChild(icon);
        heading.appendChild(info);

        const summary = document.createElement("summary");
        summary.appendChild(heading);

        const body = document.createElement("div");
        body.classList.add("comment-body");

        content = renderMarkdown(comment.text);
        body.innerHTML = `
            ${content}
            <button type="button" data-toggle="reply-form" data-target="comment-${comment.id}-reply-form">Reply</button>

            <!-- Reply form start -->
            <form class="reply-form d-none" id="comment-${comment.id}-reply-form" data-parent-id="${comment.id}">
                <textarea placeholder="Reply to comment" rows="4"></textarea>
                <button type="submit">Submit</button>
                <button type="button" data-toggle="reply-form" data-target="comment-${comment.id}-reply-form">Cancel</button>
            </form>
            <!-- Reply form end -->
        `;

        const link = document.createElement("a");
        link.classList.add("comment-border-link");

        const details = document.createElement("details");
        details.classList.add("comment");
        details.setAttribute("open", "");
        details.id = `comment-${comment.id}`;
        details.appendChild(link);
        details.appendChild(summary);
        details.appendChild(body);

        /*const collapseBtn = document.createElement("button");
        collapseBtn.textContent = "-";
        collapseBtn.onclick = () => {
            const body = wrap.querySelector(".body");
            const replies = wrap.querySelector(".replies");
            const hidden = body.style.display === "none";
            body.style.display = hidden ? "" : "none";
            if (replies) replies.style.display = hidden ? "" : "none";
            collapseBtn.textContent = hidden ? "-" : "+";
        };*/

        // replies
        if (comment.replies?.length) {
            const replies = document.createElement("div");
            replies.className = "replies";

            comment.replies.forEach(r => {
                replies.appendChild(renderComment(r));
            });

            details.appendChild(replies);
        }

        return details;
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

    function setSubmitListeners() {
        document.querySelectorAll("form.reply-form").forEach(form => {
            form.addEventListener("submit", async (e) => {
                e.preventDefault();
                let form = e.currentTarget.closest("form");
                const textarea = form.querySelector("textarea");
                const parent = form.getAttribute("data-parent-id");
                const text = textarea.value;
                textarea.value = "";
                //const temp = optimisticAdd(text, null);

                try {
                    await postComment(text, parent === null ? null : parseInt(parent));
                    await fetchComments();
                } catch (err) {
                    console.error(err);
                    //alert("Failed");
                    //removeOptimistic(temp.id);
                }
            });
        });
    }

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

    async function postComment(text, parent_id) {
        const url = document.location.href;
        await fetch(`${CONFIG['api-url']}/comments`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                url,
                text,
                parent_id
            })
        });
    }

    function renderMarkdown(text) {
        let content = escapeHtml(text).split('\n');
        for (i = 0; i < content.length; ++i) {
            let line = content[i].trim();
            if (line.length == 0)
                continue;
            line = line.replace(/\*\*([^*]+?)\*\*/g, "<b>$1</b>")
                .replace(/\*([^*]+?)\*/g, "<i>$1</i>")
                .replace(/`([^`]+?)`/g, "<code>$1</code>");
            content[i] = `<p>${line}</p>`;
        }
        return content.join("");
    }

    function escapeHtml(str) {
        return String(str)
            .replaceAll("&", "&amp;")
            .replaceAll("<", "&lt;")
            .replaceAll(">", "&gt;");
    }

    setSubmitListeners();
    fetchComments();
})();
