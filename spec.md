# context
For my development workflow I mostly stay in the terminal using claude, bash, lazygit and lazydocker for fast iterative development. I am often debugging systems locally and over a network and need to view various resources such as containers, files, git changes etc, so that I can iterate over the development loop.

# problem

I can't remember all the aws commands and it slows me down asking claude, I want a TUI for viewing AWS resources what https://github.com/jesseduffield/lazygit is for git and https://github.com/jesseduffield/lazydocker is for docker. I need to be able to read the all values for S3, api gateway, iam, secretsmanager, lambda, SNS, SQS where the initial page lists and then drills down into the value, take deep inspiration from lazy git and lazy docker as they are also built in golang

# design

exactly like lazygit / lazydocker with ability to tab through diffent panes for the different resources. AWS resources listed on the left with a list on the right then ability to view the value of the item

# issues

- route32 add entry, failed not asking for anything more than name, there are likely other required params. Check all the actions for required or optional parameters that users likely will want to set
- upload file timeout for S3
- kinesis stream create fails
- the three errors above showed no error logs in the log file, make sure aws sdk usage is being tracked. Log any request made to AWS using the SDK and ensure any errors are logged