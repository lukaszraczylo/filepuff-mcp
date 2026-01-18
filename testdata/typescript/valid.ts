/**
 * Represents a user in the system.
 */
interface User {
    id: number;
    name: string;
    email: string;
}

/**
 * Configuration options for the application.
 */
type Config = {
    debug: boolean;
    timeout: number;
};

/**
 * Greeting service for handling user greetings.
 */
class GreetingService {
    private prefix: string;

    /**
     * Creates a new GreetingService.
     * @param prefix The greeting prefix
     */
    constructor(prefix: string) {
        this.prefix = prefix;
    }

    /**
     * Greets a user.
     * @param user The user to greet
     * @returns The greeting message
     */
    greet(user: User): string {
        return `${this.prefix}, ${user.name}!`;
    }
}

/**
 * Formats a user for display.
 * @param user The user to format
 * @returns Formatted string
 */
function formatUser(user: User): string {
    return `${user.name} <${user.email}>`;
}

const DEFAULT_TIMEOUT = 5000;

export { User, Config, GreetingService, formatUser, DEFAULT_TIMEOUT };
