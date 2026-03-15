# context
For my development workflow I mostly stay in the terminal using claude, bash, lazygit and lazydocker for fast iterative development. I am often debugging systems locally and over a network and need to view various resources such as containers, files, git changes etc, so that I can iterate over the development loop.

# problem

I can't remember all the aws commands and it slows me down asking claude, I want a TUI for viewing AWS resources what https://github.com/jesseduffield/lazygit is for git and https://github.com/jesseduffield/lazydocker is for docker. I need to be able to read the all values for S3, api gateway, iam, secretsmanager, lambda, SNS, SQS where the initial page lists and then drills down into the value, take deep inspiration from lazy git and lazy docker as they are also built in golang

# design

exactly like lazygit / lazydocker with ability to tab through diffent panes for the different resources. AWS resources listed on the left with a list on the right then ability to view the value of the item
