# Remind
*A Readwise Email Clone*

Remind is a Go script that clones what I liked best about Readwise, particularly the daily email reminder of your Kindle highlights.
This script is run using a GitHub Actions Workflow triggered by a cron schedule.
Feel free to utilize this script for your personal use by following the instructions below.

## Prerequisites

Before using this script, make sure you have the following prerequisites in place:

- A valid [SendGrid](https://sendgrid.com/) API key
- A Kindle clippings file in the standard format

## Getting Started

1. Click on the "[Use this template](https://github.com/joekleinsorge/remind/generate)" button to create a new repository based on this template.
2. Configure the necessary GitHub Actions secrets for your repository:

    ```bash
    SENDGRID_API_KEY: Your SendGrid API key
    SENDER_EMAIL: The email address used as the sender for the reminders
    RECIPIENT_EMAIL: The email address where the reminders will be sent
    ```

3. Replace the example `clippings.txt` file with your own Kindle clippings file.
4. Customize the [remind.yml](/.github/workflows/remind.yml) GitHub Actions Workflow to run according to your desired schedule.

Enjoy the daily dose of REMINDer from your Kindle highlights!

