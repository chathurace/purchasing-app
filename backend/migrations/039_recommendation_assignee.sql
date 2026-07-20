-- Assignee on a recommendation's legal/security approval cards. Any member of the
-- card's team (or finance) may set it; only the assignee may approve the card
-- (comments stay open to the whole team). The budget card has no assignee.
-- ON DELETE SET NULL so deactivating/removing a user leaves the card, unassigned.

ALTER TABLE pr_recommendation_approvals
    ADD COLUMN assignee_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
