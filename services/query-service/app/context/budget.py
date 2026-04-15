class ContextBudget:
    def __init__(self, token_budget: int) -> None:
        self.token_budget = token_budget
        self.used_estimated_tokens = 0

    def can_add(self, text: str) -> bool:
        return self.used_estimated_tokens + self.estimate_tokens(text) <= self.token_budget

    def add(self, text: str) -> None:
        self.used_estimated_tokens += self.estimate_tokens(text)

    def estimate_tokens(self, text: str) -> int:
        return max(1, len(text) // 4)
